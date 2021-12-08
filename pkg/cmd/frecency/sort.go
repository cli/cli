package frecency

import "time"

type ByFrecency []entryWithStats

func (f ByFrecency) Len() int {
	return len(f)
}
func (f ByFrecency) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f ByFrecency) Less(i, j int) bool {
	iScore := f[i].Stats.Score()
	jScore := f[j].Stats.Score()
	if iScore == jScore {
		return f[i].Stats.LastAccess.After(f[j].Stats.LastAccess)
	}
	return iScore > jScore
}

func (c countEntry) Score() int {
	if c.Count == 0 {
		return 0
	}
	duration := time.Since(c.LastAccess)
	recencyScore := 10
	if duration < 1*time.Hour {
		recencyScore = 100
	} else if duration < 6*time.Hour {
		recencyScore = 80
	} else if duration < 24*time.Hour {
		recencyScore = 60
	} else if duration < 3*24*time.Hour {
		recencyScore = 40
	} else if duration < 7*24*time.Hour {
		recencyScore = 20
	}
	return recencyScore * c.Count
}
