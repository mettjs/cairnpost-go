package cairnpost

import "iter"

// walk goes through a paged list a page at a time: page answers the items
// after a cursor ("" for the first page) and the cursor of the page that
// follows, "" when there is none. It stops at the first error, which it
// yields.
func walk[T any](page func(after string) ([]T, string, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		after := ""
		for {
			items, next, err := page(after)
			if err != nil {
				var none T
				yield(none, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			// A cursor that does not move would be walked for ever.
			if next == "" || next == after {
				return
			}
			after = next
		}
	}
}
