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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ks-tool/awg-admin/storage/boltdb/dump"

	bolt "go.etcd.io/bbolt"
)

// awg-migrate moves awg-admin's data between two boltdb files — typically
// the ~/.awg-admin used by the Wails desktop app on one machine and the one
// used by the standalone web server (cmd/awg-admin.go) on another. Both
// already share the exact same file format (same boltdb buckets, same JSON
// value encoding — see storage/boltdb), so this tool doesn't need to know
// anything about the application's schema (models.Server, models.User,
// ...): it just copies every bucket's raw key/value pairs to/from a
// JSON file, byte for byte. That also means it isn't tripped up by the
// historical bktUsers/bktInterfaces bucket-name mismatch in
// storage/boltdb/bbolt.go — it doesn't care what a bucket is named or what
// its values mean, only that the source and destination are the same
// awg-admin version (the binary format of values is an internal,
// undocumented implementation detail, not a stable cross-version contract).
//
// Usage:
//
//	awg-migrate export -db ~/.awg-admin -out dump.json
//	awg-migrate import -db ~/.awg-admin -in dump.json [-force]
func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "export":
		runExportCmd(os.Args[2:])
	case "import":
		runImportCmd(os.Args[2:])
	case "-h", "-help", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "awg-migrate: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `awg-migrate: export/import awg-admin's boltdb between machines or
between the Wails desktop app and the standalone web server — both read
the same file format, so a dump taken from one can be imported into the
other.

Usage:
  awg-migrate export -db <path> -out <dump.json>
  awg-migrate import -db <path> -in <dump.json> [-force]

Flags:
  -db     path to the boltdb file (the desktop app and standalone server
          both default to ~/.awg-admin)
  -out    (export) path to write the JSON dump to
  -in     (import) path to read the JSON dump from
  -force  (import) overwrite keys that already exist in the destination
          instead of aborting on the first conflict
`)
}

func runExportCmd(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbPath := fs.String("db", "", "path to the source boltdb file")
	outPath := fs.String("out", "", "path to write the JSON dump to")
	_ = fs.Parse(args)

	if *dbPath == "" || *outPath == "" {
		fs.Usage()
		os.Exit(2)
	}

	if err := exportDB(*dbPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "export failed:", err)
		os.Exit(1)
	}
	fmt.Printf("exported %s -> %s\n", *dbPath, *outPath)
}

func runImportCmd(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dbPath := fs.String("db", "", "path to the destination boltdb file")
	inPath := fs.String("in", "", "path to read the JSON dump from")
	force := fs.Bool("force", false, "overwrite keys that already exist instead of aborting")
	_ = fs.Parse(args)

	if *dbPath == "" || *inPath == "" {
		fs.Usage()
		os.Exit(2)
	}

	if err := importDB(*dbPath, *inPath, *force); err != nil {
		fmt.Fprintln(os.Stderr, "import failed:", err)
		os.Exit(1)
	}
	fmt.Printf("imported %s -> %s\n", *inPath, *dbPath)
}

// The dump JSON shape and the copy logic live in storage/boltdb/dump, shared
// with the in-app backup feature so a dump taken by either can be restored by
// the other. This CLI only owns opening the file and streaming to/from disk.

func exportDB(dbPath, outPath string) error {
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{ReadOnly: true, Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := dump.Export(db, f); err != nil {
		return err
	}
	return f.Close()
}

func importDB(dbPath, inPath string, force bool) error {
	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}
	defer func() { _ = f.Close() }()

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	return dump.Import(db, f, force)
}
