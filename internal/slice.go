package internal

func Unique[T comparable](s []T) []T {
	results := []T{}
	seenItems := NewSet[T]()
	for _, item := range s {
		if !seenItems.Has(item) {
			results = append(results, item)
			seenItems.Add(item)
		}
	}
	return results
}

func Reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
