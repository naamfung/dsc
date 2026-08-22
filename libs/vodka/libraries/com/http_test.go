// Copyright 2013 com authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package com

import (
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// 网络集成测试探针：example.com 不可达时跳过，避免无网络环境下每次
// go test ./... 报错干扰；单次探测，包级缓存结果。
var (
	exampleOnce sync.Once
	exampleOK   bool
)

func requireExampleDotCom(t *testing.T) {
	t.Helper()
	exampleOnce.Do(func() {
		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get("http://example.com")
		if err == nil {
			resp.Body.Close()
			exampleOK = true
		}
	})
	if !exampleOK {
		t.Skip("example.com not reachable (network integration test)")
	}
}

// assertExamplePage 校验响应确实来自 example.com（只检查标题与长度，
// 不绑定页面字节数——上游硬编码的精确前缀/长度断言早已随页面改版失效）。
func assertExamplePage(t *testing.T, p []byte) {
	t.Helper()
	s := string(p)
	if len(p) == 0 || !strings.Contains(s, "<title>Example Domain</title>") {
		t.Errorf("expected example.com page with title, got %d bytes: %s", len(p), s)
	}
}

func TestHttpGet(t *testing.T) {
	requireExampleDotCom(t)
	rc, err := HttpGet(&http.Client{}, "http://example.com", nil)
	if err != nil {
		t.Fatalf("HttpGet:\n Expect => %v\n Got => %s\n", nil, err)
	}
	p, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Errorf("HttpGet:\n Expect => %v\n Got => %s\n", nil, err)
	}
	assertExamplePage(t, p)
}

func TestHttpGetBytes(t *testing.T) {
	requireExampleDotCom(t)
	p, err := HttpGetBytes(&http.Client{}, "http://example.com", nil)
	if err != nil {
		t.Errorf("HttpGetBytes:\n Expect => %v\n Got => %s\n", nil, err)
	}
	assertExamplePage(t, p)
}

func TestHttpGetJSON(t *testing.T) {

}

type rawFile struct {
	name   string
	rawURL string
	data   []byte
}

func (rf *rawFile) Name() string {
	return rf.name
}

func (rf *rawFile) RawUrl() string {
	return rf.rawURL
}

func (rf *rawFile) Data() []byte {
	return rf.data
}

func (rf *rawFile) SetData(p []byte) {
	rf.data = p
}

func TestFetchFiles(t *testing.T) {
	requireExampleDotCom(t)
	files := []RawFile{
		&rawFile{rawURL: "http://example.com"},
		&rawFile{rawURL: "http://example.com"},
	}
	err := FetchFiles(&http.Client{}, files, nil)
	if err != nil {
		t.Errorf("FetchFiles:\n Expect => %v\n Got => %s\n", nil, err)
	} else {
		assertExamplePage(t, files[0].Data())
		assertExamplePage(t, files[1].Data())
	}
}

func TestFetchFilesCurl(t *testing.T) {
	requireExampleDotCom(t)
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available (network integration test)")
	}
	files := []RawFile{
		&rawFile{rawURL: "http://example.com"},
		&rawFile{rawURL: "http://example.com"},
	}
	err := FetchFilesCurl(files)
	if err != nil {
		t.Errorf("FetchFilesCurl:\n Expect => %v\n Got => %s\n", nil, err)
	} else {
		assertExamplePage(t, files[0].Data())
		assertExamplePage(t, files[1].Data())
	}
}
