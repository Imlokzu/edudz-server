# edudz server extensions

This is the self-hosted backend for [edudz Android](https://github.com/Imlokzu/edudz).
It preserves the upstream server and the existing school compatibility fixes.

Authenticated extensions:

- `GET /api/school-day`: separate bell periods, actual breaks/free periods,
  current lesson/break, seconds remaining, school start/end, and the next actual
  school day. Includes 60 days ahead for weekends and holidays. Timetables are
  cached per account for one minute; clock state is evaluated on each request.
  `date=YYYY-MM-DD` and read-only `at=RFC3339` support schedule previews.
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
  "model": "nvidia/nemotron-3-super-120b-a12b",
  "vision_model": "meta/llama-3.2-11b-vision-instruct",
  "request_options": {
    "chat_template_kwargs": {"enable_thinking": false},
    "temperature": 0.6,
    "max_tokens": 4096
  }
}
```

Set your own key in the private file. `EDUDZ_AI_CONFIG` can point to another path.
No secrets belong in this repository or in the APK. Sampling/reasoning options can be configured separately for text and vision using `request_options` and `vision_request_options`; authenticated messages, tool definitions and stream mode remain protected. Normal school login resolves
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
go test -race ./cmd/server/assistant ./cmd/server/schoolday ./edupage/model
go test ./edupage -run TestAllowedAttachmentURL
go run github.com/swaggo/swag/cmd/swag@v1.16.1 init -g server.go -d cmd/server,edupage,icanteen --parseDependency --parseInternal
go build -o ep2-server ./cmd/server
```

Upstream's credential-dependent EduPage integration tests require its username and
password flags. The new assistant tests use a mock model API and check fragmented
streaming, tool calls, interrupted responses, history injection and image reads.

The server loads `config.yaml` as before. `HOST` and `PORT` can run a staging copy.
School time uses `EDUDZ_SCHOOL_TIMEZONE`, the account's published timezone, or
`Europe/Berlin` for this deployment. The response includes school wall time and
UTC offset so the app's clock can follow the server.
The existing Mac service uses port 8130 and ep2.waveio.me. A staging copy on 8131
was used before updating the service.

Sources: EdupageAPI/edupage-api for the day-plan protocol, ledongthuc/pdf for PDF
text extraction, and Android's MediaStore for the client-side Downloads flow.

## Stronger assistant model — 2026-10-07

The main model is NVIDIA Nemotron 3 Super 120B, selected after a live Ukrainian
school-deadline and quadratic-equation check. It excluded finished work and the
later biology deadline and found roots 2 and 3. The tested reply completed in
2.4 seconds. Vision transcription remains a separate worker; the main model
interprets its extracted material. The model configuration is read per request,
so changing the model does not require an Android update.

Reasoning deltas are preserved internally across tool calls for providers that
require them, but are not streamed to the student. Client-provided reasoning is
rejected. These behaviors have a mock-provider regression test.

DeepSeek-compatible backends can set `omit_tool_choice: true` for thinking-mode
requests while retaining tool definitions and prior assistant reasoning. The
active server stays on Nemotron until a DeepSeek provider passes live checks.

## School-day clock — 2.2

Double lessons retain the teacher, room, group and original block identity while
being split into individual bell periods. If bell metadata is unavailable, exact
multiples of 45 minutes use a 45-minute fallback. Contiguous periods do not get
invented breaks. Missing scheduled periods are marked as free time and never add
to the lesson count. The assistant's timetable uses the same splitting function.

Boundary tests cover 08:44:59 → 08:45, both school breaks and 13:00 completion.
Authenticated staging checks confirmed six lessons on 2026-10-07, Friday → Monday,
and the autumn holiday jump from 2026-10-30 to 2026-11-09.
