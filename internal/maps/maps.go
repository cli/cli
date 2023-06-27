package maps

// Map a function over all values in the map, allocating a new map for the results.
func Map[M ~map[K]V, K comparable, V any, U any](m M, f func(V) U) map[K]U {
	r := make(map[K]U, len(m))
	for k, v := range m {
		r[k] = f(v)
	}
	return r
}
