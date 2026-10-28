package handlers

import (
	"sync"
	"time"
)

// pushQueue sends one recipient's pushes in the order they were raised.
//
// Every message push used to be its own goroutine. Each one does a beta check,
// a mute check, a name lookup and a subscription read before it reaches Expo,
// so two messages a few milliseconds apart raced through those lookups and
// arrived on the phone in whichever order the database happened to answer. A
// conversation read top to bottom in the notification tray was sometimes out
// of order with the thread it came from.
//
// One FIFO per recipient fixes that without serialising everybody behind one
// slow push: different recipients still go in parallel, and only the pushes
// bound for the same person wait for each other. A recipient's worker exits
// after a quiet spell, so the map holds only people with something in flight.
//
// Enqueue order is the order fanOutMessage reaches the queue, which is a few
// queries after the insert commits — hydrating the message, the sender's name
// in a direct thread, the member list. Two sends whose requests overlap inside
// that window can still swap, whether from two people in a group or from one
// person whose app sent a photo and its follow-up text together. Sends that do
// not overlap arrive in the order they were stored.
//
// The zero value is ready to use, so AppService needs no wiring for it.
type pushQueue struct {
	mu     sync.Mutex
	queues map[string]chan func()
}

// pushQueueDepth bounds how far one recipient's pushes can back up. Past it a
// push is sent unordered rather than dropped or allowed to block the request
// that raised it — arriving out of order is better than not arriving.
const pushQueueDepth = 256

// pushQueueIdle is how long a recipient's worker waits for more before exiting.
const pushQueueIdle = 30 * time.Second

func (q *pushQueue) enqueue(recipientId string, job func()) {
	q.mu.Lock()
	if q.queues == nil {
		q.queues = map[string]chan func(){}
	}
	ch, ok := q.queues[recipientId]
	if !ok {
		ch = make(chan func(), pushQueueDepth)
		q.queues[recipientId] = ch
		go q.drain(recipientId, ch)
	}
	// Sent while holding the lock, and never blocking, so that drain's idle
	// check below cannot retire a channel between this lookup and this send.
	select {
	case ch <- job:
		q.mu.Unlock()
	default:
		q.mu.Unlock()
		go job()
	}
}

func (q *pushQueue) drain(recipientId string, ch chan func()) {
	idle := time.NewTimer(pushQueueIdle)
	defer idle.Stop()
	for {
		select {
		case job := <-ch:
			job()
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(pushQueueIdle)
		case <-idle.C:
			q.mu.Lock()
			if len(ch) == 0 {
				delete(q.queues, recipientId)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			idle.Reset(pushQueueIdle)
		}
	}
}
