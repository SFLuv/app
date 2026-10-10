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

// params is a Promise in Next 15. 15.2.6 still accepted the old synchronous
// shape; 15.5.24 enforces it, so this awaits rather than reads through. Same
// value, same rendered output — only the signature changed.
export default async function WorkflowPhotoPage({
  params,
}: {
  params: Promise<{ photo_id: string }>
}) {
  const { photo_id: rawPhotoId } = await params
  const photoId = decodeURIComponent(rawPhotoId || "").trim()

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
