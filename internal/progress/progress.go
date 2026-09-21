package progress

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Bar struct {
	mu       sync.Mutex
	total    int64
	current  int64
	started  time.Time
	label    string
	width    int
	finished bool
}

func New(label string, total int64) *Bar {
	return &Bar{
		total:   total,
		started: time.Now(),
		label:   label,
		width:   30,
	}
}

func (b *Bar) Set(current int64) {
	b.mu.Lock()
	b.current = current
	b.mu.Unlock()
	b.render()
}

func (b *Bar) Add(n int64) {
	b.mu.Lock()
	b.current += n
	b.mu.Unlock()
	b.render()
}

func (b *Bar) Finish() {
	b.mu.Lock()
	b.finished = true
	if b.total > 0 {
		b.current = b.total
	}
	b.mu.Unlock()
	b.render()
	fmt.Println()
}

func (b *Bar) render() {
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := time.Since(b.started).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}

	speed := float64(b.current) / elapsed

	if b.total <= 0 {
		line := fmt.Sprintf("\r%s %s %s/s",
			b.label,
			formatBytes(b.current),
			formatBytes(int64(speed)),
		)
		fmt.Print(line)
		return
	}

	pct := float64(b.current) / float64(b.total)
	if pct > 1 {
		pct = 1
	}

	filled := int(pct * float64(b.width))
	if filled > b.width {
		filled = b.width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", b.width-filled)

	eta := ""
	if speed > 0 && !b.finished {
		remaining := float64(b.total-b.current) / speed
		eta = fmt.Sprintf(" ETA %s", formatDuration(time.Duration(remaining)*time.Second))
	}

	line := fmt.Sprintf("\r%s [%s] %s %s/%s %s/s%s",
		b.label,
		bar,
		formatPercent(pct),
		formatBytes(b.current),
		formatBytes(b.total),
		formatBytes(int64(speed)),
		eta,
	)

	fmt.Print(line)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatPercent(p float64) string {
	return fmt.Sprintf("%5.1f%%", p*100)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
