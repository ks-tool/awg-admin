/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package mux

import (
	"fmt"
	"net/http"
)

type Middleware func(http.Handler) http.Handler

type Mux struct {
	*http.ServeMux
	middlewares []Middleware
}

func NewServeMux(mws ...Middleware) *Mux {
	return &Mux{ServeMux: http.NewServeMux(), middlewares: mws}
}

func (m *Mux) Handle(pattern string, handler http.Handler) {
	for i := len(m.middlewares) - 1; i >= 0; i-- {
		handler = m.middlewares[i](handler)
	}
	m.ServeMux.Handle(pattern, handler)
}

func (m *Mux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(handler))
}

func (m *Mux) HandleFuncMethod(method, pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.Handle(fmt.Sprintf("%s\t%s", method, pattern), http.HandlerFunc(handler))
}

func (m *Mux) GET(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodGet, path, handler)
}

func (m *Mux) HEAD(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodHead, path, handler)
}

func (m *Mux) POST(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodPost, path, handler)
}

func (m *Mux) PUT(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodPut, path, handler)
}

func (m *Mux) DELETE(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodDelete, path, handler)
}

func (m *Mux) PATCH(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodPatch, path, handler)
}

func (m *Mux) OPTIONS(path string, handler func(http.ResponseWriter, *http.Request)) {
	m.HandleFuncMethod(http.MethodOptions, path, handler)
}
