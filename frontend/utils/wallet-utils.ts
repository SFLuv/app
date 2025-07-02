// Helper function to generate a procedural QR code based on wallet address
export function generateProceduralQrData(address: string): string {
  // In a real implementation, this would generate a unique QR code pattern
  // based on the wallet address characteristics

  // For now, we'll simulate this by adding a prefix to the address
  // that would trigger special rendering in the Citizen Wallet app
  const prefix = "sfluv://citizen/"

  // Use the first 8 characters of the address to create a unique color code
  const colorCode = address.substring(2, 10)

  // Create a version parameter based on the last 2 characters
  const version = (Number.parseInt(address.substring(address.length - 2), 16) % 5) + 1

  // Combine these elements to create a procedural QR code data string
  return `${prefix}${address}?color=${colorCode}&version=${version}`
}

// Helper function to convert wallet address to a color
export function addressToColor(address: string): string {
  // Extract a portion of the address to use as a color
  const colorHex = address.substring(2, 8)
  return `#${colorHex}`
}
