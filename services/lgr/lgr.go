package lgr

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/khaledhikmat/ai-event-processor/services/lgr/internal/handler"
	"github.com/mdobak/go-xerrors"
)

/*
Reference:
https://betterstack.com/community/guides/logging/logging-in-go/
*/
const (
	appName = "ai-event-processor"
	appVer  = "0.0.1"

	envKey      = "RUN_TIME_ENV"
	logLevelKey = "LOG_LEVEL"
)

var Logger *slog.Logger

type stackFrame struct {
	Func   string `json:"func"`
	Source string `json:"source"`
	Line   int    `json:"line"`
}

func init() {
	// Setup logger
	logLevelKey := os.Getenv(logLevelKey)
	logLevel := logLevel(logLevelKey)

	// Pretty logger is used to demonstrate the use of a custom logger handler
	// opts := handler.PrettyOptions{
	// 	SlogOpts: slog.HandlerOptions{
	// 		Level: logLevel,
	// 	},
	// }

	// handler := handler.NewPretty(os.Stdout, opts)
	// Logger = slog.New(handler)

	opts := &slog.HandlerOptions{
		AddSource:   true,
		Level:       logLevel,
		ReplaceAttr: replaceAttr, // To handle errors properly
	}

	loggerHandler := slog.NewJSONHandler(os.Stdout, opts)
	spanHandler := handler.NewSpan(loggerHandler)
	topLogger := slog.New(spanHandler)
	Logger = topLogger.With(
		slog.Group("program-info",
			slog.String("app", appName),
			slog.String("version", appVer),
		),
	)

	slog.SetDefault(Logger)
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			a.Value = fmtErr(v)
		}
	}

	return a
}

// marshalStack extracts stack frames from the error
func marshalStack(err error) []stackFrame {
	trace := xerrors.StackTrace(err)

	if len(trace) == 0 {
		return nil
	}

	frames := trace.Frames()

	s := make([]stackFrame, len(frames))

	for i, v := range frames {
		f := stackFrame{
			Source: filepath.Join(
				filepath.Base(filepath.Dir(v.File)),
				filepath.Base(v.File),
			),
			Func: filepath.Base(v.Function),
			Line: v.Line,
		}

		s[i] = f
	}

	return s
}

// fmtErr returns a slog.Value with keys `msg` and `trace`. If the error
// does not implement interface { StackTrace() errors.StackTrace }, the `trace`
// key is omitted.
func fmtErr(err error) slog.Value {
	var groupValues []slog.Attr

	groupValues = append(groupValues, slog.String("msg", err.Error()))

	frames := marshalStack(err)

	if frames != nil {
		groupValues = append(groupValues,
			slog.Any("trace", frames),
		)
	}

	return slog.GroupValue(groupValues...)
}

func logLevel(lvl string) slog.Level {
	switch lvl {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
