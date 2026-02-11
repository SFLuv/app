import { CommunityConfig } from "@citizenwallet/sdk"
import { HDNodeWallet, pbkdf2, scrypt, Wallet } from "ethers"
import { getAccount, getDecryptKdfParams, getPassword } from "./ethers"

export const parseKeyFromCWLink = async (walletLink: string): Promise<string | null> => {
  try {
    let url = new URL(walletLink)
    let walletString = url.hash.split("/")[2].split("?")[0]
    const prefix = walletString.split("-")[0]
    const pw = process.env.NEXT_PUBLIC_WEB_BURNER_PASSWORD
    if(!pw) {
      return null
    }

    switch(prefix) {
      case "v2": {
        return await parseLegacyWalletFromHash(walletString, pw)
      }
      case "v3": {
        return await parsePrivateKeyFromHash(walletString, pw)
      }
      case "v4": {
        return await parsePrivateKeyFromV4Hash(walletString, pw)
      }
      default: return null
    }
  }
  catch(error) {
    console.log(error)
    return null
  }
}


export const parsePrivateKeyFromV4Hash = async (
  encoded: string,
  walletPassword: string
): Promise<string | null> => {

  try {
    if (!encoded.startsWith("v4-")) {
      throw new Error("Invalid wallet format");
    }

    const decoded = Buffer.from(encoded.replace("v4-", ""), "base64").toString(
      "utf-8"
    );

    const [account, accountFactory, encryptedPrivateKey] = decoded.split("|");
    if (!account || !accountFactory || !encryptedPrivateKey) {
      throw new Error("Invalid wallet format");
    }

    const jsonPrivateKey = JSON.parse(encryptedPrivateKey);
    if (!!jsonPrivateKey.Crypto) {
      jsonPrivateKey.crypto = jsonPrivateKey.Crypto;
      delete jsonPrivateKey.Crypto;
    }

    const password = getPassword(walletPassword);

    const params = getDecryptKdfParams(jsonPrivateKey);
    let key: string;
    if (params.name === "pbkdf2") {
      const { salt, count, dkLen, algorithm } = params;
      key = pbkdf2(password, salt, count, dkLen, algorithm);
    } else {
      const { salt, N, r, p, dkLen } = params;
      key = await scrypt(password, salt, N, r, p, dkLen, () => {});
    }

    const keyStoreAccount = getAccount(jsonPrivateKey, key);

    return keyStoreAccount.privateKey;

  } catch (e) {
    console.error(e);
  }

  return null;
};

export const parsePrivateKeyFromHash = async (
  encoded: string,
  walletPassword: string
): Promise<string | null> => {

  try {
    if (!encoded.startsWith("v3-")) {
      throw new Error("Invalid wallet format");
    }

    const decoded = Buffer.from(encoded.replace("v3-", ""), "base64").toString(
      "utf-8"
    );

    const [account, encryptedPrivateKey] = decoded.split("|");
    if (!account || !encryptedPrivateKey) {
      throw new Error("Invalid wallet format");
    }

    const jsonPrivateKey = JSON.parse(encryptedPrivateKey);
    if (!!jsonPrivateKey.Crypto) {
      jsonPrivateKey.crypto = jsonPrivateKey.Crypto;
      delete jsonPrivateKey.Crypto;
    }

    const password = getPassword(walletPassword);

    const params = getDecryptKdfParams(jsonPrivateKey);
    let key: string;
    if (params.name === "pbkdf2") {
      const { salt, count, dkLen, algorithm } = params;
      key = pbkdf2(password, salt, count, dkLen, algorithm);
    } else {
      const { salt, N, r, p, dkLen } = params;
      key = await scrypt(password, salt, N, r, p, dkLen, () => {});
    }

    const keyStoreAccount = getAccount(jsonPrivateKey, key);

    return keyStoreAccount.privateKey;

  } catch (e) {
    console.error(e);
  }

  return null;
};

export const parseLegacyWalletFromHash = async (
  encoded: string,
  walletPassword: string
): Promise<string | null> => {

  try {
    if (!encoded.startsWith("v2-")) {
      throw new Error("Invalid wallet format");
    }

    const encryptedPrivateKey = atob(encoded.replace("v2-", ""));

    const wallet = await Wallet.fromEncryptedJson(
      encryptedPrivateKey,
      walletPassword
    );

    return wallet.privateKey;
  } catch (e) {
    console.error(e);
  }

  return null;
};

export const generateWalletHashV3 = async (
  account: string,
  wallet: HDNodeWallet | Wallet,
  walletPassword: string
): Promise<string> => {
  const encryptedPrivateKey = await wallet.encrypt(walletPassword);

  const encoded = btoa(`${account}|${encryptedPrivateKey}`);

  return `v3-${encoded}`;
};

const getV3AccountFactory = (community: CommunityConfig, alias: string) => {
  let accountFactory: string;

  switch (alias) {
    case "gratitude":
      accountFactory = "0xAE6E18a9Cd26de5C8f89B886283Fc3f0bE5f04DD";
      break;
    case "bread":
      accountFactory = "0xAE76B1C6818c1DD81E20ccefD3e72B773068ABc9";
      break;
    case "wallet.commonshub.brussels":
      accountFactory = "0x307A9456C4057F7C7438a174EFf3f25fc0eA6e87";
      break;
    case "wallet.pay.brussels":
      accountFactory = "0xBABCf159c4e3186cf48e4a48bC0AeC17CF9d90FE";
      break;
    case "wallet.sfluv.org":
      accountFactory = "0x5e987a6c4bb4239d498E78c34e986acf29c81E8e";
      break;
    default:
      accountFactory = community.primaryAccountConfig.account_factory_address;
  }

  return accountFactory;
};

export const generateWalletHashV4 = async (
  account: string,
  community: CommunityConfig,
  wallet: HDNodeWallet | Wallet,
  walletPassword: string
): Promise<string> => {
  const encryptedPrivateKey = await wallet.encrypt(walletPassword);

  const accountFactory = community.primaryAccountConfig.account_factory_address;

  const encoded = btoa(`${account}|${accountFactory}|${encryptedPrivateKey}`);

  return `v4-${encoded}`;
};

export const generateWalletHashV4FromV3 = async (
  account: string,
  community: CommunityConfig,
  wallet: HDNodeWallet | Wallet,
  walletPassword: string
): Promise<string> => {
  const encryptedPrivateKey = await wallet.encrypt(walletPassword);

  // keep original account factory for legacy accounts
  const accountFactory = getV3AccountFactory(community, community.community.alias);

  const encoded = btoa(`${account}|${accountFactory}|${encryptedPrivateKey}`);

  return `v4-${encoded}`;
};
