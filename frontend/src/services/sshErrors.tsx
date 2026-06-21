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

// Matches internal/sshclient.sshPassphraseMarker. Both transports collapse
// Go errors to a plain string (HTTP error body text, or the message of a
// thrown error from a Wails-bound method) — this substring is the only
// reliable way to recognize "needs an SSH key passphrase" across both.
const SSH_PASSPHRASE_MARKER = 'SSH_PASSPHRASE_REQUIRED';

export class SSHPassphraseRequiredError extends Error {
    constructor() {
        super('SSH key requires a passphrase');
        this.name = 'SSHPassphraseRequiredError';
    }
}

/** Throws SSHPassphraseRequiredError if message carries the backend's marker. */
export function throwIfPassphraseRequired(message: string | undefined): void {
    if (message?.includes(SSH_PASSPHRASE_MARKER)) {
        throw new SSHPassphraseRequiredError();
    }
}
