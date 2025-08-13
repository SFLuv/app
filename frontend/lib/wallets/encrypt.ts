import {Chacha20Poly1305} from '@hpke/chacha20poly1305';
import {CipherSuite, DhkemP256HkdfSha256, HkdfSha256} from '@hpke/core';
import {base64} from '@scure/base';

export const encryptWithHpke = async (
  encryptionPublicKey: string,
  plaintextPrivateKey: string
) => {
  const eBuf: ArrayBuffer = base64.decode(encryptionPublicKey).buffer as ArrayBuffer
  const pBuf: ArrayBuffer =  new Uint8Array(
    Buffer.from(
      // Be sure to remove the `0x` prefix if present
      plaintextPrivateKey.replace(/^0x/, ''),
      'hex'
    )
  ).buffer as ArrayBuffer
  // Deserialize the raw key returned by the `init` request to the Privy API to a public key object
  const suite = new CipherSuite({
    kem: new DhkemP256HkdfSha256(),
    kdf: new HkdfSha256(),
    aead: new Chacha20Poly1305()
  });
  const publicKeyObject = await suite.kem.deserializePublicKey(eBuf);

  // Encrypt the plaintext wallet private key
  const sender = await suite.createSenderContext({
    recipientPublicKey: publicKeyObject
  });
  const ciphertext = await sender.seal(pBuf);

  // Return the encapsulated key and ciphertext, converting ArrayBuffer to Uint8Array
  return {
    encapsulatedKey: new Uint8Array(sender.enc),
    ciphertext: new Uint8Array(ciphertext)
  };
};
