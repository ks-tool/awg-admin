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

import { get, post, remove } from './api';
import { getCurrentApiMode } from './apiMode';
import { bindingsClient } from './bindingsClient';
import { reportError } from './errorReporting';
import type { Peer, Key } from '@/types';

export interface AddPeerInput {
  name: string;
  interfaceId: string;
  allowedIps: string[];
  endpoint?: string;
  dns?: string[];
  privateKey?: string;
  presharedKey?: string;
  withPresharedKey?: boolean;
  keepaliveInterval?: number;
}

/**
 * Get the appropriate client based on current API mode
 */
function getClient() {
  return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Get all peers for a user
 */
export async function listPeers(userId: string): Promise<Peer[] | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.listPeers(userId);
    if (error) {
      reportError(`fetch-peers-${userId}`, `Failed to fetch peers for user ${userId}`, error);
      return null;
    }
    return data as unknown as Peer[];
  }

  const { data, error } = await get<Peer[]>(`/users/${userId}/peers/`);
  if (error) {
    reportError(`fetch-peers-${userId}`, `Failed to fetch peers for user ${userId}`, error);
    return null;
  }
  return data;
}

/**
 * Get a single peer by public key
 */
export async function getPeer(userId: string, publicKey: Key): Promise<Peer | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.getPeer(userId, publicKey);
    if (error) {
      reportError(`fetch-peer-${publicKey}`, `Failed to fetch peer ${publicKey}`, error);
      return null;
    }
    return data as unknown as Peer;
  }

  const { data, error } = await get<Peer>(`/users/${userId}/peers/${publicKey}`);
  if (error) {
    reportError(`fetch-peer-${publicKey}`, `Failed to fetch peer ${publicKey}`, error);
    return null;
  }
  return data;
}

/**
 * Get a wg-quick style client config for a peer, for QR-code/download
 * provisioning.
 */
export async function getPeerConfig(userId: string, publicKey: Key): Promise<string | null> {
  const client = getClient();

  if (client) {
    const { data, error } = await client.getPeerConfig(userId, publicKey);
    if (error) {
      reportError(`fetch-peer-config-${publicKey}`, `Failed to fetch config for peer ${publicKey}`, error);
      return null;
    }
    return data;
  }

  const { data, error } = await get<{ config: string }>(`/users/${userId}/peers/${publicKey}?format=config`);
  if (error) {
    reportError(`fetch-peer-config-${publicKey}`, `Failed to fetch config for peer ${publicKey}`, error);
    return null;
  }
  return data.config;
}

/**
 * Get a peer's client config rendered as a QR code (base64-encoded PNG),
 * for provisioning a phone/desktop WireGuard client by scanning. Rendered
 * server-side (see Service.GetPeerQRCode) so the config text — which
 * embeds the peer's private key — never has to pass through the browser
 * as a string a JS QR library would need to hold onto.
 */
export async function getPeerQRCode(userId: string, publicKey: Key): Promise<string | null> {
  const client = getClient();

  if (client) {
    const { data, error } = await client.getPeerQRCode(userId, publicKey);
    if (error) {
      reportError(`fetch-peer-qrcode-${publicKey}`, `Failed to fetch QR code for peer ${publicKey}`, error);
      return null;
    }
    return data;
  }

  const { data, error } = await get<{ qrcode: string }>(`/users/${userId}/peers/${publicKey}?format=qrcode`);
  if (error) {
    reportError(`fetch-peer-qrcode-${publicKey}`, `Failed to fetch QR code for peer ${publicKey}`, error);
    return null;
  }
  return data.qrcode;
}

/**
 * Save a peer's QR code to a PNG file.
 *
 * The two API modes need different mechanisms: the Wails desktop webview
 * can't download a `data:` URL (an `<a download>` is a silent no-op there),
 * so desktop routes through a native save dialog in Go (`SavePeerQRCode`).
 * The plain browser (web-server mode / dev) already holds the rendered PNG as
 * `qrDataUrl`, so it just triggers an anchor download locally — no server
 * round-trip. Returns true if a file was saved (or the download was
 * triggered), false if the user cancelled the desktop save dialog or an error
 * was reported.
 */
export async function savePeerQRCode(
  userId: string,
  publicKey: Key,
  defaultName: string,
  qrDataUrl: string,
): Promise<boolean> {
  const client = getClient();

  if (client) {
    const { data, error } = await client.savePeerQRCode(userId, publicKey, defaultName);
    if (error) {
      reportError(`save-peer-qrcode-${publicKey}`, `Failed to save QR code for peer ${publicKey}`, error);
      return false;
    }
    return data;
  }

  const a = document.createElement('a');
  a.href = qrDataUrl;
  a.download = `${defaultName}.png`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  return true;
}

/**
 * Add a peer to a user
 */
export async function addPeer(userId: string, input: AddPeerInput): Promise<boolean> {
  const client = getClient();
  
  if (client) {
    const { error } = await client.addPeer(userId, input);
    if (error) {
      console.error(`Failed to add peer to user ${userId} (bindings):`, error);
      return false;
    }
    return true;
  }
  
  const { error } = await post<void, AddPeerInput>(`/users/${userId}/peers`, input);
  if (error) {
    console.error(`Failed to add peer to user ${userId}:`, error);
    return false;
  }
  return true;
}

/**
 * Delete a peer
 */
export async function deletePeer(userId: string, publicKey: Key): Promise<boolean> {
  const client = getClient();
  
  if (client) {
    const { error } = await client.deletePeer(userId, publicKey);
    if (error) {
      console.error(`Failed to delete peer ${publicKey} (bindings):`, error);
      return false;
    }
    return true;
  }
  
  const { error } = await remove<void>(`/users/${userId}/peers/${publicKey}`);
  if (error) {
    console.error(`Failed to delete peer ${publicKey}:`, error);
    return false;
  }
  return true;
}
