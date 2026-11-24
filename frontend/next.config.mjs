/** @type {import('next').NextConfig} */
const nextConfig = {
  eslint: {
    ignoreDuringBuilds: true,
  },
  typescript: {
    ignoreBuildErrors: true,
  },
  images: {
    unoptimized: true,
  },
  async redirects() {
    return [
      {
        source: "/",
        destination: "/map",
        permanent: false,
      },
      // The merchant application moved here, and the website and old emails
      // still link to where it was: without this they land on a 404.
      {
        source: "/settings/merchant-approval",
        destination: "/merchant-onboarding",
        permanent: false,
      },
    ]
  },
}

export default nextConfig
