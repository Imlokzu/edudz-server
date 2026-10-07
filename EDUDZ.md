# edudz server extensions

This is the self-hosted backend for [edudz Android](https://github.com/Imlokzu/edudz).
It preserves the upstream server and the existing school compatibility fixes.

Authenticated extensions:

- `GET /api/lesson-plan?date=YYYY-MM-DD`: the student's published lesson topics.
- `GET /api/etest?testid=…&superid=…`: homework material cards.
- `GET /api/file?src=…`: downloads the student's EduPage attachments using the session.
  Only HTTPS `edupage.org` hosts are allowed, including every redirect.
- `POST /api/assistant`: a real SSE stream with bounded, read-only school tools.
  `status`, `sources`, `token`, `error`, `done` events and keepalive comments.
- `GET /api/assistant/status`: whether the model is configured.

The assistant reads schedules, rooms, homework, grades, messages, published topics,
PDF/Office/text attachments and images. It does not submit work or send messages.
The student's school credentials and the model API key are never included in model
context. Chat/history validation, per-account rate limits and a concurrent-request
limit apply. School messages and attachments are treated as source data.

## Model configuration

Put a private JSON file at `~/.config/edudz-ai/config.json`, permissions 0600:

```json
{
  "base_url": "https://integrate.api.nvidia.com/v1",
  "api_key": "",
  "model": "meta/llama-3.2-90b-vision-instruct",
  "vision_model": "meta/llama-3.2-11b-vision-instruct"
}
```

Set your own key in the private file. `EDUDZ_AI_CONFIG` can point to another path.
No secrets belong in this repository or in the APK. Normal school login resolves
the school automatically for global EduPage accounts; a school is optional.

For scanned PDF pages on the current Mac server:

```sh
mkdir -p ~/.config/edudz-ai
swiftc tools/pdf-preview.swift -o ~/.config/edudz-ai/pdf-preview
```

The PDFKit helper renders a window of up to four pages. The assistant can request
later pages. `EDUDZ_PDF_RENDERER` can select another compatible renderer. Text
extraction works without the helper; image-only PDFs then require a renderer.

## Build and validation

```sh
go test ./cmd/server/assistant ./edupage/model
go test ./edupage -run TestAllowedAttachmentURL
go build -o ep2-server ./cmd/server
```

Upstream's credential-dependent EduPage integration tests require its username and
password flags. The new assistant tests use a mock model API and check fragmented
streaming, tool calls, interrupted responses, history injection and image reads.

The server loads `config.yaml` as before. `HOST` and `PORT` can run a staging copy.
The existing Mac service uses port 8130 and ep2.waveio.me. A staging copy on 8131
was used before updating the service.

Sources: EdupageAPI/edupage-api for the day-plan protocol, ledongthuc/pdf for PDF
text extraction, and Android's MediaStore for the client-side Downloads flow.
