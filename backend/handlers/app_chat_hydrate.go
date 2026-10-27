package handlers

import (
	"context"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

// Turning stored messages into what a client renders.
//
// Four things are added on the way out, and all four are batched across the
// whole page rather than fetched per message: attachments, their references,
// reactions, and the quoted slice of anything replied to. A fifty-message page
// costs four queries regardless of how much is on it.
//
// Reference resolution is the interesting half. An attachment that points at a
// transaction, a merchant, an event or a support ticket stores only the pointer,
// and what it points at lives somewhere this package has to go and ask — the
// chain index, the locations table, the bot database, the support queue. Any of
// those can have changed or gone since the message was sent, so each resolver
// either fills the card in or marks it Unavailable. Nothing fails the page.

// hydrateMessages fills in everything a page of messages needs beyond its own
// columns. Safe to call with an empty page.
func (a *AppService) hydrateMessages(ctx context.Context, conv *structs.ConversationRow, msgs []*structs.Message, readerId string) {
	if len(msgs) == 0 {
		return
	}

	ids := make([]int64, 0, len(msgs))
	replyTargets := []int64{}
	for _, m := range msgs {
		// A deleted message has nothing to hydrate: its body is already blanked
		// and loading its attachments or reactions would hand back content it no
		// longer has. Its reply target is still resolved, because the tombstone
		// keeps its place in the conversation.
		if !m.Deleted {
			ids = append(ids, m.Id)
		}
		if m.ReplyToId != nil {
			replyTargets = append(replyTargets, *m.ReplyToId)
		}
	}

	if attachments, err := a.db.GetMessageAttachments(ctx, ids); err == nil {
		for _, m := range msgs {
			if cards, found := attachments[m.Id]; found {
				a.resolveAttachmentReferences(ctx, cards, readerId)
				m.Attachments = cards
			}
		}
	} else {
		a.logger.Logf("error loading attachments for conversation %d: %s", conv.Id, err)
	}

	if reactions, err := a.db.GetMessageReactions(ctx, ids, readerId); err == nil {
		for _, m := range msgs {
			m.Reactions = reactions[m.Id]
		}
	} else {
		a.logger.Logf("error loading reactions for conversation %d: %s", conv.Id, err)
	}

	if len(replyTargets) > 0 {
		if quotes, err := a.db.GetMessageQuotes(ctx, replyTargets); err == nil {
			for _, m := range msgs {
				if m.ReplyToId != nil {
					// Absent means deleted. The reply keeps its reply_to_id and
					// renders without a quote, rather than disappearing with
					// the message it answered.
					m.ReplyTo = quotes[*m.ReplyToId]
				}
			}
		} else {
			a.logger.Logf("error loading reply quotes for conversation %d: %s", conv.Id, err)
		}
	}

	// Only groups need a name on each bubble. A direct thread's header already
	// says who the other person is, and repeating it on every bubble is noise.
	if conv.IsGroup() {
		senders := map[string]struct{}{}
		for _, m := range msgs {
			senders[m.SenderId] = struct{}{}
		}
		ids := make([]string, 0, len(senders))
		for id := range senders {
			ids = append(ids, id)
		}
		if names, err := a.db.GetSenderNames(ctx, ids); err == nil {
			for _, m := range msgs {
				if name := names[m.SenderId]; name != "" {
					m.SenderName = name
				} else {
					m.SenderName = "SFLuv neighbor"
				}
			}
		}
	}
}

// resolveAttachmentReferences fills in the live detail behind each reference
// card, or marks it unavailable.
func (a *AppService) resolveAttachmentReferences(ctx context.Context, cards []structs.MessageAttachment, readerId string) {
	for i := range cards {
		card := &cards[i]
		switch card.Kind {
		case structs.AttachmentKindTransaction:
			a.resolveTransactionCard(ctx, card)
		case structs.AttachmentKindMerchant:
			a.resolveMerchantCard(ctx, card)
		case structs.AttachmentKindEvent:
			a.resolveEventCard(ctx, card)
		case structs.AttachmentKindSupport:
			a.resolveSupportCard(ctx, card, readerId)
		}
	}
}

// resolveTransactionCard looks a transfer up in the chain index.
//
// ── What is deliberately NOT returned ───────────────────────────────────────
//
// The memo. A memo is what the sender wrote for their own records, stored
// against the hash in our database and shown to them on the transaction's own
// screen. It is not public, and attaching a transaction to a chat is not a
// decision to publish it — somebody sharing "here is the payment I made" is
// sharing the amount and the parties, which are on a public chain anyway, not a
// private note they wrote about it. So this reads the indexer, which has no
// memos, rather than our transactions table, which does.
func (a *AppService) resolveTransactionCard(ctx context.Context, card *structs.MessageAttachment) {
	if a.ponderDb == nil || card.TxHash == "" {
		card.Unavailable = true
		return
	}
	transfer, err := a.ponderDb.GetTransferByHash(ctx, card.TxHash)
	if err != nil || transfer == nil {
		// Not indexed yet is the common case for a transfer attached seconds
		// ago, and is indistinguishable here from one that never happened. The
		// card says "confirming" either way and resolves on the next read.
		card.Unavailable = true
		return
	}
	amount := transfer.Amount
	card.TxAmountSfluvBase = &amount
	card.TxFrom = transfer.From
	card.TxTo = transfer.To
	// The indexer stores the block timestamp unsigned; the card carries it
	// signed, because every other time in this API is a signed unix second.
	timestamp := int64(transfer.Timestamp)
	card.TxAt = &timestamp
	card.Unavailable = false
}

// resolveMerchantCard looks up an approved location.
//
// Unapproved or removed locations are treated as gone: a merchant card is a
// pointer at the public map, and anything not on the map should not be reachable
// through a chat message either.
func (a *AppService) resolveMerchantCard(ctx context.Context, card *structs.MessageAttachment) {
	if card.LocationId == nil {
		card.Unavailable = true
		return
	}
	location, err := a.db.GetLocation(ctx, uint64(*card.LocationId))
	if err != nil || location == nil || !location.Approval {
		card.Unavailable = true
		return
	}
	card.LocationName = location.Name
	card.LocationAddr = strings.TrimSpace(strings.Join(
		[]string{location.Street, location.City, location.State}, ", "))
	lat, lng := location.Lat, location.Lng
	card.LocationLat = &lat
	card.LocationLng = &lng
	card.Unavailable = false
}

// resolveEventCard looks up a volunteer event in the bot database.
//
// An event that has finished or been cancelled still resolves, with its title
// and times, and is marked unavailable so the card renders as past rather than
// as a live link to sign up for something that is over.
func (a *AppService) resolveEventCard(ctx context.Context, card *structs.MessageAttachment) {
	if card.EventId == "" {
		card.Unavailable = true
		return
	}
	event, live := a.lookupLinkedEvent(ctx, card.EventId)
	if event == nil {
		card.Unavailable = true
		return
	}
	card.EventTitle = event.Title
	start, end := event.StartAt, event.EndAt
	card.EventStart = &start
	card.EventEnd = &end
	card.Unavailable = !live
}

// resolveSupportCard looks up a support ticket.
//
// ── Who may see what ────────────────────────────────────────────────────────
//
// A support ticket holds somebody's message to us and their email address. The
// card is therefore deliberately thin: reference, subject and category, never
// the body and never the email. Those are in the admin panel, behind an admin
// check, and a chat message is not the place to widen that.
//
// It resolves for anybody in the conversation, which is the point — the whole
// feature is a staff member opening a chat with the person who wrote in, and
// both ends need to see which ticket they are talking about.
func (a *AppService) resolveSupportCard(ctx context.Context, card *structs.MessageAttachment, readerId string) {
	if card.SupportRequestId == nil {
		card.Unavailable = true
		return
	}
	request, err := a.db.GetSupportRequest(ctx, *card.SupportRequestId)
	if err != nil || request == nil {
		card.Unavailable = true
		return
	}
	card.SupportReference = request.Reference
	card.SupportSubject = request.Subject
	card.SupportCategory = request.Category
	card.Unavailable = false
}
