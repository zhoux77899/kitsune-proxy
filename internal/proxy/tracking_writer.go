package proxy

import (
	"net/http"
)

type trackingWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
}

func (w *trackingWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackingWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytesWritten += int64(written)
	return written, err
}

func (w *trackingWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *trackingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *trackingWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *trackingWriter) BytesWritten() int64 {
	return w.bytesWritten
}
