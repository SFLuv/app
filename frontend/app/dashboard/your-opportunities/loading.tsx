export default function Loading() {
  return (
    <div className="space-y-6">
      <div>
        <div className="h-8 w-64 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse mb-2"></div>
        <div className="h-4 w-96 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
      </div>

      <div className="flex justify-between items-center">
        <div className="h-10 w-64 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
      </div>

      <div className="rounded-md border bg-white dark:bg-[#2a2a2a]">
        <div className="grid grid-cols-12 gap-4 p-4 font-medium border-b">
          <div className="col-span-5 h-6 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
          <div className="col-span-2 h-6 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
          <div className="col-span-2 h-6 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
          <div className="col-span-3 h-6 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
        </div>

        {Array.from({ length: 5 }).map((_, index) => (
          <div key={index} className="grid grid-cols-12 gap-4 p-4 border-b">
            <div className="col-span-5 flex items-center">
              <div className="h-10 w-10 rounded-md bg-gray-200 dark:bg-gray-700 animate-pulse mr-3"></div>
              <div className="space-y-2">
                <div className="h-5 w-40 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
                <div className="h-4 w-60 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
              </div>
            </div>
            <div className="col-span-2 flex items-center">
              <div className="h-5 w-20 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
            </div>
            <div className="col-span-2 flex items-center">
              <div className="h-5 w-16 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
            </div>
            <div className="col-span-3 flex items-center">
              <div className="h-6 w-24 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
