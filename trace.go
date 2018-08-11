package dataloader

import (
	"context"
	"fmt"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"
)

// Tracer is an interface that may be used to implement tracing.
type Tracer interface {
	// Load handles tracing for calls to the Load function
	Load(context.Context, Key) (context.Context, LoadFinishFunc)
	// LoadMany handles tracing for calls to the LoadMany function
	LoadMany(context.Context, []Key) (context.Context, LoadManyFinishFunc)
	// Batch handles tracing for calls to the BatchFunction function
	Batch(context.Context) (context.Context, BatchFinishFunc)
}

type (
	// LoadFinishFunc finishes the tracing for the Load function
	LoadFinishFunc func(Result)
	// LoadManyFinishFunc finishes the tracing for the LoadMany function
	LoadManyFinishFunc func(ResultMap)
	// BatchFinishFunc finishes the tracing for the BatchFunction
	BatchFinishFunc func(ResultMap)
)

// ======================================= no-op tracing implementation ======================================

// NewNoOpTracer returns an instance of a blank tracer with no action
func NewNoOpTracer() Tracer {
	return &noOpTracer{}
}

type noOpTracer struct{}

func (*noOpTracer) Load(ctx context.Context, _ Key) (context.Context, LoadFinishFunc) {
	return ctx, func(Result) {}
}

func (*noOpTracer) LoadMany(ctx context.Context, _ []Key) (context.Context, LoadManyFinishFunc) {
	return ctx, func(ResultMap) {}
}

func (*noOpTracer) Batch(ctx context.Context) (context.Context, BatchFinishFunc) {
	return ctx, func(ResultMap) {}
}

// ======================================= open tracing implementation =======================================

// NewOpenTracingTracer returns an instance of a tracer conforming to the open tracing standard
func NewOpenTracingTracer() Tracer {
	return &openTracer{}
}

type openTracer struct{}

func (*openTracer) Load(ctx context.Context, key Key) (context.Context, LoadFinishFunc) {
	span, spanCtx := opentracing.StartSpanFromContext(ctx, "Dataloader: load")
	span.SetTag("dataloader.key", key.String())

	return spanCtx, func(Result) { span.Finish() }
}

func (*openTracer) LoadMany(ctx context.Context, keyArr []Key) (context.Context, LoadManyFinishFunc) {
	span, spanCtx := opentracing.StartSpanFromContext(ctx, "Dataloader: loadMany")

	keys := make([]string, 0, len(keyArr))
	for _, k := range keyArr {
		keys = append(keys, k.String())
	}
	span.SetTag("dataloader.keys", keys)

	return spanCtx, func(ResultMap) {}
}

func (*openTracer) Batch(ctx context.Context) (context.Context, BatchFinishFunc) {
	span, spanCtx := opentracing.StartSpanFromContext(ctx, "Dataloader: batch")

	return spanCtx, func(r ResultMap) {
		span.SetTag("keys", fmt.Sprintf("[%s]", strings.Join(r.Keys(), ", ")))
		span.Finish()
	}
}
