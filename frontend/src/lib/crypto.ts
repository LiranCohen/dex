import * as ed25519 from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { hkdf } from '@noble/hashes/hkdf.js';
import { validateMnemonic as validateBip39, mnemonicToSeedSync, generateMnemonic } from '@scure/bip39';
import { wordlist as englishWordlist } from '@scure/bip39/wordlists/english.js';

// Configure ed25519 to use sha512 synchronously
ed25519.hashes.sha512 = (message: Uint8Array) => sha512(message);

/**
 * Validate a BIP39 mnemonic phrase
 */
export function validateMnemonic(mnemonic: string): boolean {
  return validateBip39(mnemonic, englishWordlist);
}

/**
 * Generate a new 24-word BIP39 mnemonic
 */
export function generatePassphrase(): string {
  return generateMnemonic(englishWordlist, 256); // 256 bits = 24 words
}

// Text encoder for converting strings to bytes
const textEncoder = new TextEncoder();

/**
 * Derive an Ed25519 keypair from a BIP39 mnemonic
 * Uses HKDF with sha512 to derive key material, matching the backend implementation
 */
export function deriveKeypair(mnemonic: string): { publicKey: Uint8Array; privateKey: Uint8Array } {
  // Convert mnemonic to seed (empty passphrase for Poindexter auth)
  const seed = mnemonicToSeedSync(mnemonic, '');

  // Use HKDF to derive 32 bytes for Ed25519 seed
  // Parameters must match backend: salt="poindexter-auth", info="ed25519-keypair"
  const salt = textEncoder.encode('poindexter-auth');
  const info = textEncoder.encode('ed25519-keypair');
  const ed25519Seed = hkdf(sha512, seed, salt, info, 32);

  // Generate Ed25519 keypair from seed
  const privateKey = ed25519Seed;
  const publicKey = ed25519.getPublicKey(privateKey);

  return { publicKey, privateKey };
}

/**
 * Sign a message with an Ed25519 private key
 */
export function sign(message: Uint8Array, privateKey: Uint8Array): Uint8Array {
  return ed25519.sign(message, privateKey);
}

/**
 * Convert bytes to hex string
 */
export function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map(b => b.toString(16).padStart(2, '0'))
    .join('');
}

/**
 * Convert hex string to bytes
 */
export function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.substring(i, i + 2), 16);
  }
  return bytes;
}
