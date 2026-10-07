package assistant

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// The Mac server uses a PDFKit helper; unsupported hosts still get PDF text.
func renderPDF(ctx context.Context, body []byte, start int) ([]string, error) {
	home, _ := os.UserHomeDir()
	binary := os.Getenv("EDUDZ_PDF_RENDERER")
	if binary == "" {
		binary = filepath.Join(home, ".config", "edudz-ai", "pdf-preview")
	}
	if _, err := os.Stat(binary); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "edudz-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "attachment.pdf")
	if err = os.WriteFile(file, body, 0600); err != nil {
		return nil, err
	}
	sub, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(sub, binary, file, dir, strconv.Itoa(start))
	if err = cmd.Run(); err != nil {
		return nil, err
	}
	images := []string{}
	for page := start; page < start+4; page++ {
		b, e := os.ReadFile(filepath.Join(dir, fmt.Sprintf("page-%d.png", page)))
		if e != nil {
			break
		}
		if len(b) > 5<<20 {
			return nil, fmt.Errorf("page image too large")
		}
		images = append(images, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(b))
	}
	return images, nil
}
