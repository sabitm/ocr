# ocr

A CLI tool that extracts text from images using PaddleOCR via a Gradio endpoint.

## Usage

```bash
./ocr [--dns <server>] <image-path>
```

The `--dns` flag lets you specify a custom DNS server (e.g. `8.8.8.8`) for host resolution.

Set `DEBUG=1` for debug logging.
