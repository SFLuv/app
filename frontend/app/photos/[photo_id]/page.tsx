import type { Metadata } from "next"

import { BACKEND } from "@/lib/constants"

export const metadata: Metadata = {
  title: "Workflow Photo",
  robots: {
    index: false,
    follow: false,
    nocache: true,
  },
}

export default function WorkflowPhotoPage({
  params,
}: {
  params: { photo_id: string }
}) {
  const photoId = decodeURIComponent(params.photo_id || "").trim()

  if (!photoId) {
    return (
      <main className="min-h-screen bg-black text-white flex items-center justify-center p-6">
        <p className="text-sm text-white/70">Photo not found.</p>
      </main>
    )
  }

  const photoSrc = `${BACKEND}/workflow-photos/public/${encodeURIComponent(photoId)}`

  return (
    <main className="min-h-screen bg-black flex items-center justify-center p-4 sm:p-6">
      <img
        src={photoSrc}
        alt="Workflow photo"
        className="max-h-[95vh] max-w-full object-contain"
      />
    </main>
  )
}
