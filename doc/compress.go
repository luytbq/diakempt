package doc

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"io"
	"strings"
	"unicode/utf8"
)

// draw.io compresses a page as base64(deflateRaw(encodeURIComponent(xml))).

func decompressPage(text string) (string, error) {
	raw, err := decodeBase64(text)
	if err != nil {
		return "", err
	}
	inflated, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return "", err
	}
	return decodeURIComponent(string(inflated)), nil
}

func compressPage(xml string) string {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write([]byte(encodeURIComponent(xml)))
	w.Close()
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// decodeBase64 skips characters outside the alphabet, such as line breaks that
// editors insert, and accepts missing padding.
func decodeBase64(s string) ([]byte, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/' {
			b.WriteByte(c)
		}
	}
	return base64.RawStdEncoding.DecodeString(b.String())
}

const uriUnreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"

func encodeURIComponent(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(uriUnreserved, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// decodeURIComponent decodes %XX escapes. A malformed escape is kept as written
// and invalid UTF-8 becomes U+FFFD, where the browser function would throw.
func decodeURIComponent(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				out = append(out, hi<<4|lo)
				i += 2
				continue
			}
		}
		out = append(out, s[i])
	}
	if !utf8.Valid(out) {
		return strings.ToValidUTF8(string(out), "�")
	}
	return string(out)
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
