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

package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	appconfig "github.com/ks-tool/app-config/go"
	"github.com/spf13/pflag"
)

func Load(cfg any, prefix string, args []string) error {
	if err := appconfig.Load(cfg,
		appconfig.Env(),
		appconfig.Options(appconfig.EnvOptions{Prefix: prefix}),
	); err != nil {
		return err
	}
	flags := pflag.NewFlagSet(prefix, pflag.ExitOnError)
	appconfig.BindFlags(flags, prefix, cfg)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	return nil
}

func TLS(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && clientCAFile == "" {
		return nil, nil
	}
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return nil, errors.New("TLS_CERT, TLS_KEY and TLS_CLIENT_CA must all be set together")
	}

	caPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("client CA file contains no valid certificates")
	}

	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		MinVersion: tls.VersionTLS12,
	}, nil
}
