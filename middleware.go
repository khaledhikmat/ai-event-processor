package main

import (
	"net/http"
	"time"

	"github.com/khaledhikmat/ai-event-processor/services/lgr"
)

// ResponseWriter wraps http.ResponseWriter to capture status code.
type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// LoggingMiddleware logs the details of each request.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &ResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			timesince := time.Since(start)
			str := timesince.String()

			if ww.statusCode >= 400 {
				lgr.Logger.Warn("Request handled",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.statusCode,
					"duration", str,
					"remote_addr", r.RemoteAddr,
				)
			}
			if ww.statusCode >= 500 {
				lgr.Logger.Error("Request handled",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.statusCode,
					"duration", str,
					"remote_addr", r.RemoteAddr,
				)
			}
		}()

		next.ServeHTTP(ww, r)
	})
}

// TraceMiddleware propagates Cloud Trace context.
// func TraceMiddleware(projectID string, next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		traceHeader := r.Header.Get("X-Cloud-Trace-Context")
// 		traceParts := strings.Split(traceHeader, "/")
// 		if len(traceParts) > 0 && len(traceParts[0]) > 0 {
// 			traceID := traceParts[0]
// 			var trace string
// 			if projectID != "" {
// 				trace = fmt.Sprintf("projects/%s/traces/%s", projectID, traceID)
// 			} else {
// 				trace = traceID
// 			}
// 			ctx := logging.AddTraceToContext(r.Context(), trace)
// 			next.ServeHTTP(w, r.WithContext(ctx))
// 			return
// 		}
// 		next.ServeHTTP(w, r)
// 	})
// }
