// keepStable returns the PREVIOUS value when it is deeply equal (by JSON) to the
// next value, so React state that is re-fetched on background refreshes keeps
// the same reference and does not trigger a re-render / visual flash when the
// data hasn't actually changed. Fall back to `next` if either side can't be
// serialized.
//
// Use it in data setters that run on polling/focus/visibility refreshes:
//   setItems((prev) => keepStable(prev, data.items ?? []))
export const keepStable = <T>(current: T, next: T): T => {
  try {
    return JSON.stringify(current) === JSON.stringify(next) ? current : next
  } catch {
    return next
  }
}
