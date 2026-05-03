package web

import "fmt"

// BytesHumanRu форматирует размер в байтах для UI (целые единицы, вниз по ступеням KiB..TiB).
func BytesHumanRu(n int64) string {
	if n <= 0 {
		return "0 Б"
	}
	const (
		KiB = int64(1024)
		MiB = KiB * 1024
		GiB = MiB * 1024
		TiB = GiB * 1024
	)
	switch {
	case n < KiB:
		return fmt.Sprintf("%d Б", n)
	case n < MiB:
		return fmt.Sprintf("%d КБ", n/KiB)
	case n < GiB:
		return fmt.Sprintf("%d МБ", n/MiB)
	case n < TiB:
		return fmt.Sprintf("%d ГБ", n/GiB)
	default:
		return fmt.Sprintf("%d ТБ", n/TiB)
	}
}
