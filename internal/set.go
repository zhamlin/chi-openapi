package internal

func NewSet[K comparable]() Set[K] {
	return map[K]struct{}{}
}

type Set[K comparable] map[K]struct{}

func (s Set[K]) Add(k K) {
	s[k] = struct{}{}
}

func (s Set[K]) Del(k K) {
	delete(s, k)
}

func (s Set[K]) Has(k K) bool {
	_, has := s[k]
	return has
}

func (s Set[K]) Items() []K {
	items := make([]K, 0, len(s))
	for item := range s {
		items = append(items, item)
	}
	return items
}
