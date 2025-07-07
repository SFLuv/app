export default function MerchantApprovalLoading() {
  return (
    <div className="max-w-3xl mx-auto">
      <div className="h-8 w-64 bg-gray-200 dark:bg-gray-700 rounded-md mb-6 animate-pulse"></div>
      <div className="h-4 w-full bg-gray-200 dark:bg-gray-700 rounded-md mb-6 animate-pulse"></div>

      <div className="border rounded-lg p-6 bg-white dark:bg-gray-800">
        <div className="h-6 w-48 bg-gray-200 dark:bg-gray-700 rounded-md mb-2 animate-pulse"></div>
        <div className="h-4 w-64 bg-gray-200 dark:bg-gray-700 rounded-md mb-8 animate-pulse"></div>

        <div className="space-y-6">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="space-y-2">
              <div className="h-4 w-32 bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
              <div className="h-10 w-full bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
            </div>
          ))}

          <div className="h-10 w-full bg-gray-200 dark:bg-gray-700 rounded-md animate-pulse"></div>
        </div>
      </div>
    </div>
  )
}
