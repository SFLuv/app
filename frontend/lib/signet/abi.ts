/** Minimal ABIs for the Signet resolver stack and the Safe owner check. */

export const gateAbi = [
  {
    type: "function", name: "isAllowed", stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [{ name: "", type: "bool" }],
  },
  {
    type: "function", name: "allowAll", stateMutability: "view",
    inputs: [], outputs: [{ name: "", type: "bool" }],
  },
  {
    type: "function", name: "owner", stateMutability: "view",
    inputs: [], outputs: [{ name: "", type: "address" }],
  },
  {
    type: "function", name: "setAllowlisted", stateMutability: "nonpayable",
    inputs: [
      { name: "accounts", type: "address[]" },
      { name: "allowed", type: "bool" },
    ],
    outputs: [],
  },
] as const

export const registryAbi = [
  {
    type: "function", name: "safeFor", stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [{ name: "", type: "address" }],
  },
  {
    // Caller IS the account. Unusable for Privy EOAs, which hold no CELO.
    type: "function", name: "bind", stateMutability: "nonpayable",
    inputs: [{ name: "safe", type: "address" }],
    outputs: [],
  },
  {
    // The production path: the account's EIP-712 signature is the authority,
    // and any relayer may submit it. Only the account can ever authorize a
    // binding — there is no admin path, so this cannot be prefilled from the
    // users table.
    type: "function", name: "bindWithSignature", stateMutability: "nonpayable",
    inputs: [
      { name: "account", type: "address" },
      { name: "safe", type: "address" },
      { name: "deadline", type: "uint256" },
      { name: "signature", type: "bytes" },
    ],
    outputs: [],
  },
  {
    type: "function", name: "nonces", stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [{ name: "", type: "uint256" }],
  },
] as const

export const resolverAbi = [
  {
    type: "function", name: "resolve", stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [
      { name: "ok", type: "bool" },
      { name: "subject", type: "bytes32" },
    ],
  },
  {
    type: "function", name: "typeAndVersion", stateMutability: "pure",
    inputs: [], outputs: [{ name: "", type: "string" }],
  },
] as const

/** The Safe surface the resolver and the enrolment step depend on. */
export const safeAbi = [
  {
    type: "function", name: "isOwner", stateMutability: "view",
    inputs: [{ name: "owner", type: "address" }],
    outputs: [{ name: "", type: "bool" }],
  },
  {
    type: "function", name: "getOwners", stateMutability: "view",
    inputs: [], outputs: [{ name: "", type: "address[]" }],
  },
  {
    type: "function", name: "addOwnerWithThreshold", stateMutability: "nonpayable",
    inputs: [
      { name: "owner", type: "address" },
      { name: "_threshold", type: "uint256" },
    ],
    outputs: [],
  },
] as const
