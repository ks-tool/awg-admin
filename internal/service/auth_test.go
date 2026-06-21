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

package service

import "testing"

func TestBasicAuthDisabledByDefaultAndLetsEverythingThrough(t *testing.T) {
	svc := newTestService(t)

	enabled, err := svc.BasicAuthEnabled()
	if err != nil {
		t.Fatalf("BasicAuthEnabled: %v", err)
	}
	if enabled {
		t.Fatal("expected basic auth to be off by default")
	}

	// With it off, VerifyBasicAuth must let any request through, even with
	// no/garbage credentials — there's nothing to check.
	ok, err := svc.VerifyBasicAuth("", "")
	if err != nil {
		t.Fatalf("VerifyBasicAuth: %v", err)
	}
	if !ok {
		t.Fatal("expected requests to pass when basic auth is disabled")
	}
}

func TestBasicAuthEnabledChecksTheAdminAccount(t *testing.T) {
	svc := newTestService(t)

	if err := svc.SetBasicAuthEnabled(true); err != nil {
		t.Fatalf("SetBasicAuthEnabled: %v", err)
	}
	enabled, err := svc.BasicAuthEnabled()
	if err != nil {
		t.Fatalf("BasicAuthEnabled: %v", err)
	}
	if !enabled {
		t.Fatal("expected basic auth to be on after SetBasicAuthEnabled(true)")
	}

	// storage/boltdb seeds admin/admin on first run.
	ok, err := svc.VerifyBasicAuth("admin", "admin")
	if err != nil {
		t.Fatalf("VerifyBasicAuth: %v", err)
	}
	if !ok {
		t.Fatal("expected the seeded admin/admin credentials to pass")
	}

	ok, err = svc.VerifyBasicAuth("admin", "wrong-password")
	if err != nil {
		t.Fatalf("VerifyBasicAuth: %v", err)
	}
	if ok {
		t.Fatal("expected a wrong password to fail")
	}

	ok, err = svc.VerifyBasicAuth("not-admin", "admin")
	if err != nil {
		t.Fatalf("VerifyBasicAuth: %v", err)
	}
	if ok {
		t.Fatal("expected a wrong username to fail")
	}

	// Toggling it back off must immediately let unauthenticated requests
	// through again — same account, just the gate itself disabled.
	if err := svc.SetBasicAuthEnabled(false); err != nil {
		t.Fatalf("SetBasicAuthEnabled(false): %v", err)
	}
	ok, err = svc.VerifyBasicAuth("", "")
	if err != nil {
		t.Fatalf("VerifyBasicAuth: %v", err)
	}
	if !ok {
		t.Fatal("expected requests to pass again once basic auth is disabled")
	}
}
