package middleware

import (
	"io"
	"strings"
	"sync"

	"compress/gzip"
	"net/http"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"discobox/internal/types"
)

// compressionWriter wraps response writer with compression
type compressionWriter struct {
	http.ResponseWriter
	writer io.WriteCloser
}

func (cw *compressionWriter) Write(b []byte) (int, error) {
	return cw.writer.Write(b)
}

func (cw *compressionWriter) Close() error {
	return cw.writer.Close()
}

// Compression creates compression middleware
func Compression(config types.ProxyConfig) types.Middleware {
	cfg := config.Middleware.Compression

	// Create set of compressible types
	compressibleTypes := make(map[string]bool)
	for _, t := range cfg.Types {
		compressibleTypes[t] = true
	}

	// Create set of enabled algorithms
	enabledAlgorithms := make(map[string]bool)
	for _, algo := range cfg.Algorithms {
		enabledAlgorithms[algo] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if client accepts compression
			acceptEncoding := r.Header.Get("Accept-Encoding")
			if acceptEncoding == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Determine best encoding
			var encoding string
			var writer io.WriteCloser

			// Priority order: br, zstd, gzip
			if strings.Contains(acceptEncoding, "br") && enabledAlgorithms["br"] {
				encoding = "br"
				writer = brotli.NewWriterLevel(w, cfg.Level)
			} else if strings.Contains(acceptEncoding, "zstd") && enabledAlgorithms["zstd"] {
				encoder, _ := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(cfg.Level)))
				encoding = "zstd"
				writer = encoder
			} else if strings.Contains(acceptEncoding, "gzip") && enabledAlgorithms["gzip"] {
				encoding = "gzip"
				gzWriter, _ := gzip.NewWriterLevel(w, cfg.Level)
				writer = gzWriter
			}

			if writer == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap response writer
			w.Header().Set("Content-Encoding", encoding)
			w.Header().Del("Content-Length") // Remove content length as it will change

			cw := &compressionWriter{
				ResponseWriter: w,
				writer:         writer,
			}
			defer cw.Close()

			// Capture response to check content type
			rw := &responseWriter{
				ResponseWriter: cw,
				compressible:   compressibleTypes,
				shouldCompress: false,
			}

			next.ServeHTTP(rw, r)
		})
	}
}

// responseWriter captures response to check if compression should be applied
type responseWriter struct {
	http.ResponseWriter
	compressible   map[string]bool
	shouldCompress bool
	wroteHeader    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		contentType := rw.Header().Get("Content-Type")
		if contentType != "" {
			// Extract main type
			if idx := strings.Index(contentType, ";"); idx != -1 {
				contentType = contentType[:idx]
			}
			contentType = strings.TrimSpace(contentType)

			// Check if type is compressible
			rw.shouldCompress = rw.compressible[contentType]
		}
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	if rw.shouldCompress {
		return rw.ResponseWriter.Write(b)
	}

	// Write directly without compression
	if cw, ok := rw.ResponseWriter.(*compressionWriter); ok {
		return cw.ResponseWriter.Write(b)
	}

	return rw.ResponseWriter.Write(b)
}

// CompressionPool manages compression writers with pooling
type CompressionPool struct {
	gzipPool *sync.Pool
	brPool   *sync.Pool
	zstdPool *sync.Pool
	level    int
}

// NewCompressionPool creates a new compression pool
func NewCompressionPool(level int) *CompressionPool {
	return &CompressionPool{
		level: level,
		gzipPool: &sync.Pool{
			New: func() any {
				w, _ := gzip.NewWriterLevel(nil, level)
				return w
			},
		},
		brPool: &sync.Pool{
			New: func() any {
				return brotli.NewWriterLevel(nil, level)
			},
		},
		zstdPool: &sync.Pool{
			New: func() any {
				encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
				return encoder
			},
		},
	}
}

// GetGzip returns a gzip writer from the pool
func (cp *CompressionPool) GetGzip(w io.Writer) *gzip.Writer {
	gz := cp.gzipPool.Get().(*gzip.Writer)
	gz.Reset(w)
	return gz
}

// PutGzip returns a gzip writer to the pool
func (cp *CompressionPool) PutGzip(gz *gzip.Writer) {
	gz.Reset(nil)
	cp.gzipPool.Put(gz)
}

// GetBrotli returns a brotli writer from the pool
func (cp *CompressionPool) GetBrotli(w io.Writer) *brotli.Writer {
	br := cp.brPool.Get().(*brotli.Writer)
	br.Reset(w)
	return br
}

// PutBrotli returns a brotli writer to the pool
func (cp *CompressionPool) PutBrotli(br *brotli.Writer) {
	br.Reset(nil)
	cp.brPool.Put(br)
}

// GetZstd returns a zstd encoder from the pool
func (cp *CompressionPool) GetZstd(w io.Writer) *zstd.Encoder {
	enc := cp.zstdPool.Get().(*zstd.Encoder)
	enc.Reset(w)
	return enc
}

// PutZstd returns a zstd encoder to the pool
func (cp *CompressionPool) PutZstd(enc *zstd.Encoder) {
	enc.Reset(nil)
	cp.zstdPool.Put(enc)
}
