//go:build !grpcnotrace

/*
 *
<<<<<<<< HEAD:vendor/google.golang.org/grpc/internal/grpcsync/oncefunc.go
 * Copyright 2022 gRPC authors.
========
 * Copyright 2024 gRPC authors.
>>>>>>>> main:vendor/google.golang.org/grpc/trace_withtrace.go
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

<<<<<<<< HEAD:vendor/google.golang.org/grpc/internal/grpcsync/oncefunc.go
package grpcsync

import (
	"sync"
)

// OnceFunc returns a function wrapping f which ensures f is only executed
// once even if the returned function is executed multiple times.
func OnceFunc(f func()) func() {
	var once sync.Once
	return func() {
		once.Do(f)
	}
========
package grpc

import (
	"context"

	t "golang.org/x/net/trace"
)

func newTrace(family, title string) traceLog {
	return t.New(family, title)
}

func newTraceContext(ctx context.Context, tr traceLog) context.Context {
	return t.NewContext(ctx, tr)
}

func newTraceEventLog(family, title string) traceEventLog {
	return t.NewEventLog(family, title)
>>>>>>>> main:vendor/google.golang.org/grpc/trace_withtrace.go
}
