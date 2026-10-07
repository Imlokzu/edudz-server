package edupage

import "testing"

func TestAllowedAttachmentURL(t *testing.T) {
	for _, raw := range []string{"https://school.edupage.org/cloud/file.pdf", "https://cloud-1.edupage.org/a.png"} {
		if !allowedAttachmentURL(raw) {
			t.Fatal("valid attachment blocked")
		}
	}
	for _, raw := range []string{"https://edupage.org.evil.example/secret", "https://evil-edupage.org/a", "http://school.edupage.org/a", "https://edupage.org:8130/a", "https://user:pass@edupage.org/a", "http://127.0.0.1/"} {
		if allowedAttachmentURL(raw) {
			t.Fatal("invalid attachment allowed")
		}
	}
}
