export default function Loading() {
  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <div className="h-8 w-64 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
          <div className="h-4 w-96 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
        </div>
        <div className="flex space-x-2">
          <div className="h-10 w-24 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
          <div className="h-10 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
        </div>
      </div>

      <div className="w-full">
        <div className="h-10 w-full bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-6"></div>

        <div className="bg-white dark:bg-[#2a2a2a] rounded-lg border p-6">
          <div className="space-y-6">
            <div className="flex justify-end">
              <div className="h-10 w-28 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
            </div>

            <div className="space-y-4">
              <div>
                <div className="h-6 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
                <div className="h-20 w-full bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
              </div>

              <div className="grid grid-cols-2 gap-6">
                <div>
                  <div className="h-6 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
                  <div className="h-6 w-48 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
                </div>

                <div>
                  <div className="h-6 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
                  <div className="h-6 w-64 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-6">
                <div>
                  <div className="h-6 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
                  <div className="h-6 w-40 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
                </div>

                <div>
                  <div className="h-6 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
                  <div className="h-6 w-56 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
