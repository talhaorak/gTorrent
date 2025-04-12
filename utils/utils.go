package utils

import (
	"os"
	"strconv"
)

func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return strconv.FormatFloat(float64(bytes)/float64(TB), 'f', 2, 64) + " TB"
	case bytes >= GB:
		return strconv.FormatFloat(float64(bytes)/float64(GB), 'f', 2, 64) + " GB"
	case bytes >= MB:
		return strconv.FormatFloat(float64(bytes)/float64(MB), 'f', 2, 64) + " MB"
	case bytes >= KB:
		return strconv.FormatFloat(float64(bytes)/float64(KB), 'f', 2, 64) + " KB"
	default:
		return strconv.FormatInt(bytes, 10) + " B"
	}
}

func CopyFile(src, dst string) error {
	srContent, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	err = os.WriteFile(dst, srContent, 0644)
	if err != nil {
		return err
	}

	return nil
}
