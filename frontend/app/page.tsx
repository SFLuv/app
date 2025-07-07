"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import Image from "next/image"
import { Button } from "@/components/ui/button"
import { useApp } from "@/context/app-context"

export default function LandingPage() {
  const router = useRouter()
  const { status, user } = useApp()

  // Redirect to dashboard if already logged in
  useEffect(() => {
    if (status === "authenticated" && user) {
      router.push("/dashboard")
    }
  }, [status, user, router])

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-[#d3d3d3] dark:bg-[#1a1a1a]">
      {/* Hero Section */}
      <header className="bg-white dark:bg-[#2a2a2a] shadow-md">
        <div className="container mx-auto px-4 py-6 flex justify-between items-center">
          <div className="flex items-center">
            <Image
              src="https://hebbkx1anhila5yf.public.blob.vercel-storage.com/SFLUV%20Currency%20Symbol%20Logo-vy4PMmBMXIecSbbo0Ozx2nQVRmUyru.png"
              alt="SFLuv"
              width={120}
              height={48}
              className="h-10 w-auto"
            />
          </div>
          <div className="flex gap-4">
            <Button
              variant="ghost"
              className="text-black dark:text-white hover:bg-[#eb6c6c] hover:text-white"
              onClick={() => router.push("/login")}
            >
              Login
            </Button>
            <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={() => router.push("/signup")}>
              Sign Up
            </Button>
          </div>
        </div>
      </header>

      <section className="py-20 px-4">
        <div className="container mx-auto max-w-5xl">
          <div className="text-center mb-16">
            <h1 className="text-4xl md:text-6xl font-bold mb-6 text-black dark:text-white">
              Strengthening Local Economies
            </h1>
            <p className="text-xl md:text-2xl text-gray-700 dark:text-gray-300 max-w-3xl mx-auto">
              SFLuv is a community currency that helps keep money in the local economy, supporting small businesses and
              community initiatives.
            </p>
            <div className="mt-10">
              <Button
                className="bg-[#eb6c6c] hover:bg-[#d55c5c] text-lg px-8 py-6 h-auto"
                onClick={() => router.push("/signup")}
              >
                Get Started
              </Button>
            </div>
          </div>
        </div>
      </section>

      {/* About Section */}
      <section className="py-16 bg-white dark:bg-[#2a2a2a]">
        <div className="container mx-auto px-4">
          <div className="max-w-5xl mx-auto">
            <h2 className="text-3xl font-bold mb-8 text-black dark:text-white">About SFLuv</h2>
            <div className="grid md:grid-cols-2 gap-12">
              <div>
                <h3 className="text-xl font-semibold mb-4 text-black dark:text-white">Our Mission</h3>
                <p className="text-gray-700 dark:text-gray-300">
                  SFLuv is a community currency designed to strengthen the local economy in San Francisco. By keeping
                  money circulating within the community, we help support local businesses, create jobs, and build a
                  more resilient economy.
                </p>
              </div>
              <div>
                <h3 className="text-xl font-semibold mb-4 text-black dark:text-white">How It Works</h3>
                <p className="text-gray-700 dark:text-gray-300">
                  Community members can earn SFLuv through volunteer work and spend it at participating local
                  businesses. Merchants can accept SFLuv as payment and use it to pay other local businesses or convert
                  it to USD.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Benefits Section */}
      <section className="py-16">
        <div className="container mx-auto px-4">
          <div className="max-w-5xl mx-auto">
            <h2 className="text-3xl font-bold mb-12 text-center text-black dark:text-white">Benefits</h2>
            <div className="grid md:grid-cols-3 gap-8">
              <div className="bg-white dark:bg-[#2a2a2a] p-6 rounded-lg shadow-md">
                <h3 className="text-xl font-semibold mb-4 text-black dark:text-white">For Community Members</h3>
                <ul className="space-y-2 text-gray-700 dark:text-gray-300">
                  <li>• Earn currency through volunteer work</li>
                  <li>• Support local businesses</li>
                  <li>• Build community connections</li>
                  <li>• Participate in local economy</li>
                </ul>
              </div>
              <div className="bg-white dark:bg-[#2a2a2a] p-6 rounded-lg shadow-md">
                <h3 className="text-xl font-semibold mb-4 text-black dark:text-white">For Local Businesses</h3>
                <ul className="space-y-2 text-gray-700 dark:text-gray-300">
                  <li>• Attract new customers</li>
                  <li>• Increase customer loyalty</li>
                  <li>• Reduce cash outflow</li>
                  <li>• Support community initiatives</li>
                </ul>
              </div>
              <div className="bg-white dark:bg-[#2a2a2a] p-6 rounded-lg shadow-md">
                <h3 className="text-xl font-semibold mb-4 text-black dark:text-white">For the Community</h3>
                <ul className="space-y-2 text-gray-700 dark:text-gray-300">
                  <li>• Keep money in the local economy</li>
                  <li>• Support community projects</li>
                  <li>• Create local jobs</li>
                  <li>• Build economic resilience</li>
                </ul>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="py-20 bg-[#eb6c6c] bg-opacity-10">
        <div className="container mx-auto px-4 text-center">
          <h2 className="text-3xl font-bold mb-6 text-black dark:text-white">Join the SFLuv Community</h2>
          <p className="text-xl mb-10 max-w-3xl mx-auto text-gray-700 dark:text-gray-300">
            Whether you're a community member or a local business, SFLuv offers a way to strengthen our local economy
            together.
          </p>
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c] text-lg px-8 py-6 h-auto"
              onClick={() => router.push("/signup")}
            >
              Sign Up
            </Button>
            <Button
              variant="outline"
              className="text-black dark:text-white hover:bg-[#eb6c6c] hover:text-white text-lg px-8 py-6 h-auto"
              onClick={() => router.push("/login")}
            >
              Login
            </Button>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="bg-white dark:bg-[#2a2a2a] py-12">
        <div className="container mx-auto px-4">
          <div className="flex flex-col md:flex-row justify-between items-center">
            <div className="mb-6 md:mb-0">
              <Image
                src="https://hebbkx1anhila5yf.public.blob.vercel-storage.com/SFLUV%20Currency%20Symbol%20Logo-vy4PMmBMXIecSbbo0Ozx2nQVRmUyru.png"
                alt="SFLuv"
                width={100}
                height={40}
                className="h-8 w-auto"
              />
              <p className="mt-2 text-gray-600 dark:text-gray-400">Strengthening local economies</p>
            </div>
            <div className="text-gray-600 dark:text-gray-400">
              &copy; {new Date().getFullYear()} SFLuv. All rights reserved.
            </div>
          </div>
        </div>
      </footer>
    </div>
  )
}
