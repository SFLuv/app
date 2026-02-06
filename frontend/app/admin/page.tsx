"use client"

import { useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { useMerchants } from "@/hooks/api/use-merchants"
import { useToast } from "@/hooks/use-toast"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Calendar } from "@/components/ui/calendar"
import { cn } from "@/lib/utils"
import {
  Coins,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Users,
  FileCheck,
  Building2,
  Mail,
  Phone,
  MapPin,
  Check,
  X,
  Loader2,
  CreditCard,
  Wallet,
  Plus,
  Eye,
  EyeOff,
  QrCode,
  Download,
  CalendarIcon,
  Leaf,
} from "lucide-react"
import { useLocation } from "@/context/LocationProvider"
import { AuthedLocation, UpdateLocationApprovalRequest } from "@/types/location"
import { AppWallet } from "@/lib/wallets/wallets"
import { FAUCET_ADDRESS, SFLUV_DECIMALS, SFLUV_TOKEN } from "@/lib/constants"
import { Event, EventsStatus } from "@/types/event"
import { AddEventModal } from "@/components/events/add-event-modal"
import { EventModal } from "@/components/events/event-modal"
import { DrainFaucetModal } from "@/components/events/drain-faucet-modal"
import EventCard from "@/components/events/event-card"
import type { W9Submission } from "@/types/w9"

// Mock PayPal accounts
const mockPaypalAccounts = [
  {
    id: "paypal-1",
    email: "admin@sfluv.com",
    name: "SFLuv Admin Account",
    isVerified: true,
    isDefault: true,
  },
  {
    id: "paypal-2",
    email: "business@sfluv.com",
    name: "SFLuv Business Account",
    isVerified: true,
    isDefault: false,
  },
]




export default function AdminPage() {
  const { user, wallets, authFetch, status } = useApp()
  const { getAuthedMapLocations, updateLocationApproval, authedMapLocations} = useLocation()
  const { toast } = useToast()

  // Global wallet selection
  const [selectedWallet, setSelectedWallet] = useState<AppWallet | null>(null)
  const [selectedWalletBYUSDBalance, setSelectedWalletBYUSDBalance] = useState<number>(0)
  const [selectedWalletSFLUVBalance, setSelectedWalletSFLUVBalance] = useState<number>(0)

  const [pendingW9Submissions, setPendingW9Submissions] = useState<W9Submission[]>([])
  const [w9Loading, setW9Loading] = useState<boolean>(false)


  // Token management state
  const [amount, setAmount] = useState("")
  const [conversionType, setConversionType] = useState<"wrap" | "unwrap">("wrap")
  const [isProcessing, setIsProcessing] = useState(false)

  // PayPal conversion state
  const [paypalAmount, setPaypalAmount] = useState("")
  const [selectedPaypalAccount, setSelectedPaypalAccount] = useState<string>("")
  const [isConvertingToPaypal, setIsConvertingToPaypal] = useState(false)
  const [paypalAccounts, setPaypalAccounts] = useState(mockPaypalAccounts)

  // PayPal account modal state
  const [isPaypalModalOpen, setIsPaypalModalOpen] = useState(false)
  const [newPaypalEmail, setNewPaypalEmail] = useState("")
  const [newPaypalPassword, setNewPaypalPassword] = useState("")
  const [newPaypalFirstName, setNewPaypalFirstName] = useState("")
  const [newPaypalLastName, setNewPaypalLastName] = useState("")
  const [newPaypalPhone, setNewPaypalPhone] = useState("")
  const [newPaypalAccountName, setNewPaypalAccountName] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [isConnectingPaypal, setIsConnectingPaypal] = useState(false)

  // Merchant review modal state
  const [selectedLocationForReview, setselectedLocationForReview] = useState<any>(null)
  const [isLocationReviewModalOpen, setisLocationReviewModalOpen] = useState(false)

  // QR code generation state
  const [eventStartDate, setEventStartDate] = useState<Date>()
  const [eventEndDate, setEventEndDate] = useState<Date>()
  const [eventStartTime, setEventStartTime] = useState("")
  const [eventEndTime, setEventEndTime] = useState("")
  const [sfluvPerCode, setSfluvPerCode] = useState("")
  const [numberOfCodes, setNumberOfCodes] = useState("")
  const [isGeneratingCodes, setIsGeneratingCodes] = useState(false)
  const [generatedCodes, setGeneratedCodes] = useState<any[]>([])
  const [pendingLocations, setPendingLocations] = useState<AuthedLocation[]>([])
  const [faucetBalance, setFaucetBalance] = useState<string | bigint>("-")

  // Events
  const [events, setEvents] = useState<Event[]>([])
  const [eventsStatus, setEventsStatus] = useState<EventsStatus>("loading")
  const [eventsError, setEventsError] = useState<string>("")
  const [eventsSearch, setEventsSearch] = useState<string>("")
  const [eventsPage, setEventsPage] = useState<number>(0)
  const [eventsCount, setEventsCount] = useState<number>(10)
  const [eventsExpired, setEventsExpired] = useState<boolean>(false)
  const [eventsModalOpen, setEventsModalOpen] = useState<boolean>(false)
  const [eventDetailModalOpen, setEventDetailModalOpen] = useState<boolean>(false)
  const [deleteEventError, setDeleteEventError] = useState<string | undefined>(undefined)
  const [eventDetailsEvent, setEventDetailsEvent] = useState<Event | undefined>(undefined)
  const [drainFaucetModalOpen, setDrainFaucetModalOpen] = useState<boolean>(false)
  const [drainFaucetError, setDrainFaucetError] = useState<boolean>(false)

  const toggleNewEventModal = () => {
    setEventsModalOpen(!eventsModalOpen)
  }

  const toggleEventDetailModal = () => {
    setEventDetailModalOpen(!eventDetailModalOpen)
  }

  const toggleDrainFaucetModal = () => {
    setDrainFaucetModalOpen(!drainFaucetModalOpen)
  }

  const handleDrainFaucet = async () => {
    const url = "/drain"
    try {
      const res = await authFetch(url, {
        method: "POST"
      })
      if(res.status !== 201) throw new Error()
      setDrainFaucetError(false)
    }
    catch {
      setDrainFaucetError(true)
    }
  }

  const handleDeleteEvent = async (id: string) => {
    const url = "/events/" + id
    try {
      const res = await authFetch(url, {
        method: "DELETE",
      })
      if(res.status !== 200) throw new Error()
    }
    catch {
      setEventsStatus("error")
      setEventsError("Error adding event. Please try again later.")
    }

    await getEvents()
    toggleEventDetailModal()
  }

  const handleAddEvent = async (ev: Event) => {
    const url = "/events"
    try {
      const res = await authFetch(url, {
        method: "POST",
        body: JSON.stringify(ev)
      })
    }
    catch {
      setEventsStatus("error")
      setEventsError("Error adding event. Please try again later.")
    }

    await getEvents()
    toggleNewEventModal()
  }

  const getFaucetBalance = async () => {
    const decimals = 10 ** SFLUV_DECIMALS
    const bal = await wallets[0]?.getBalanceOf(SFLUV_TOKEN, FAUCET_ADDRESS)

    setFaucetBalance(bal ? bal / BigInt(decimals) : "-")
  }



  const getEvents = async () => {
    const url = "/events"
      + "?page=" + eventsPage
      + "&count=" + eventsCount
      + "&expired=" + eventsExpired
      + (eventsSearch ? "&search=" + eventsSearch : "")
    try {
      const res = await authFetch(url)

      const e = await res.json()
      console.log(e)
      setEvents(e)
    }
    catch {
      setEventsStatus("error")
      setEventsError("Error fetching events. Please try again later.")
    }
  }

  useEffect(() => {
    getFaucetBalance()
  }, [wallets])

  useEffect(() => {
    getAuthedMapLocations()
    getEvents()
  }, [])

  const fetchPendingW9Submissions = async () => {
    if (!user?.isAdmin) return
    setW9Loading(true)
    try {
      const res = await authFetch("/admin/w9/pending")
      if (res.status !== 200) {
        throw new Error("failed to fetch w9 submissions")
      }
      const data = await res.json()
      setPendingW9Submissions(data.submissions || [])
    } catch {
      toast({
        title: "Error",
        description: "Failed to load W9 submissions.",
        variant: "destructive",
      })
    } finally {
      setW9Loading(false)
    }
  }

  useEffect(() => {
    if (status === "authenticated" && user?.isAdmin) {
      fetchPendingW9Submissions()
    }
  }, [status, user?.isAdmin])

  const handleApproveW9 = async (id: number) => {
    try {
      const res = await authFetch("/admin/w9/approve", {
        method: "PUT",
        body: JSON.stringify({ id }),
      })
      if (res.status !== 200) {
        throw new Error("failed to approve w9")
      }
      setPendingW9Submissions((prev) => prev.filter((submission) => submission.id !== id))
      toast({
        title: "W9 Approved",
        description: "The W9 submission has been approved.",
      })
    } catch {
      toast({
        title: "Approval Failed",
        description: "Failed to approve W9 submission. Please try again.",
        variant: "destructive",
      })
    }
  }

  useEffect(() => {
      setPendingLocations(authedMapLocations.filter((location) => location.approval === null))
  }, [authedMapLocations])

  useEffect(() => {
    setSelectedWalletBalances()
  }, [selectedWallet])




  async function setSelectedWalletBalances() {
    if (selectedWallet !== null) {
    const SFLuvPromise = selectedWallet.getSFLUVBalanceFormatted()
    const BYUSDPromise = selectedWallet.getBYUSDBalanceFormatted()

    const SFLuvBalance = await SFLuvPromise
    if (SFLuvBalance != null) {
      setSelectedWalletSFLUVBalance(SFLuvBalance)
      }

    const BYUSDBalance = await BYUSDPromise
    if (BYUSDBalance != null) {
      setSelectedWalletBYUSDBalance(BYUSDBalance)
      }
    }
  }


  // Get selected PayPal account data
  const getSelectedPaypalAccount = () => {
    if (!selectedPaypalAccount) return null
    return paypalAccounts.find((account) => account.id === selectedPaypalAccount)
  }

  const selectedPaypalData = getSelectedPaypalAccount()

  // Format number to 2 decimal places and prevent negative values
  const formatAmount = (value: string): string => {
    // Remove any non-numeric characters except decimal point
    let cleaned = value.replace(/[^0-9.]/g, "")

    // Prevent multiple decimal points
    const parts = cleaned.split(".")
    if (parts.length > 2) {
      cleaned = parts[0] + "." + parts.slice(1).join("")
    }

    // Limit to 2 decimal places
    if (parts[1] && parts[1].length > 2) {
      cleaned = parts[0] + "." + parts[1].substring(0, 2)
    }

    // Prevent negative values
    if (cleaned.startsWith("-")) {
      cleaned = cleaned.substring(1)
    }

    return cleaned
  }

  const handleAmountChange = (value: string) => {
    const formatted = formatAmount(value)
    setAmount(formatted)
  }

  const handlePaypalAmountChange = (value: string) => {
    const formatted = formatAmount(value)
    setPaypalAmount(formatted)
  }

  const handleTokenConversion = async () => {
    const convertAmount = Number.parseFloat(amount)

    if (!convertAmount || convertAmount <= 0) {
      toast({
        title: "Invalid Amount",
        description: "Please enter a valid amount to convert.",
        variant: "destructive",
      })
      return
    }

    if (!selectedWallet) {
      toast({
        title: "No Wallet Selected",
        description: "Please select a wallet to perform the conversion.",
        variant: "destructive",
      })
      return
    }

    const walletData = selectedWallet
    if (!walletData) {
      toast({
        title: "Wallet Not Found",
        description: "Selected wallet could not be found.",
        variant: "destructive",
      })
      return
    }

    if (conversionType === "wrap" && convertAmount > selectedWalletBYUSDBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough BYUSD in this wallet to wrap this amount.",
        variant: "destructive",
      })
      return
    }

    if (conversionType === "unwrap" && convertAmount > selectedWalletSFLUVBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough SFLUV in this wallet to unwrap this amount.",
        variant: "destructive",
      })
      return
    }

    setIsProcessing(true)
    try {
      //Token conversion call goes here
      console.log("Token Conversion")
    } catch (error) {
      toast({
        title: "Conversion Failed",
        description: "Failed to convert tokens. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsProcessing(false)
    }
  }

  const handlePaypalConversion = async () => {
    const convertAmount = Number.parseFloat(paypalAmount)
    if (!convertAmount || convertAmount <= 0) {
      toast({
        title: "Invalid Amount",
        description: "Please enter a valid amount to convert.",
        variant: "destructive",
      })
      return
    }

    if (!selectedWallet) {
      toast({
        title: "No Wallet Selected",
        description: "Please select a wallet to cash out from.",
        variant: "destructive",
      })
      return
    }

    if (!selectedPaypalAccount) {
      toast({
        title: "No PayPal Account Selected",
        description: "Please select a PayPal account to cash out to.",
        variant: "destructive",
      })
      return
    }

    const walletData = selectedWallet
    if (!walletData) {
      toast({
        title: "Wallet Not Found",
        description: "Selected wallet could not be found.",
        variant: "destructive",
      })
      return
    }

    if (convertAmount > selectedWalletBYUSDBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough BYUSD in this wallet to convert to cash.",
        variant: "destructive",
      })
      return
    }

    const selectedAccount = paypalAccounts.find((account) => account.id === selectedPaypalAccount)

    setIsConvertingToPaypal(true)
    try {
      // PayPal offload called here
      console.log("PayPal offload")
    } catch (error) {
      toast({
        title: "PayPal Conversion Failed",
        description: "Failed to convert to PayPal cash. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsConvertingToPaypal(false)
    }
  }

  const handleConnectPaypalAccount = async () => {
    if (!newPaypalEmail || !newPaypalPassword || !newPaypalFirstName || !newPaypalLastName || !newPaypalAccountName) {
      toast({
        title: "Missing Information",
        description: "Please fill in all required PayPal account details.",
        variant: "destructive",
      })
      return
    }

    // Basic email validation
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
    if (!emailRegex.test(newPaypalEmail)) {
      toast({
        title: "Invalid Email",
        description: "Please enter a valid email address.",
        variant: "destructive",
      })
      return
    }

    // Basic phone validation (if provided)
    if (newPaypalPhone && !/^\+?[\d\s\-$$$$]+$/.test(newPaypalPhone)) {
      toast({
        title: "Invalid Phone Number",
        description: "Please enter a valid phone number.",
        variant: "destructive",
      })
      return
    }

    // Check if account already exists
    const existingAccount = paypalAccounts.find((account) => account.email === newPaypalEmail)
    if (existingAccount) {
      toast({
        title: "Account Already Exists",
        description: "This PayPal account is already connected.",
        variant: "destructive",
      })
      return
    }

    setIsConnectingPaypal(true)
    try {
      // Simulate PayPal authentication and connection API call
      await new Promise((resolve) => setTimeout(resolve, 3000))

      const newAccount = {
        id: `paypal-${Date.now()}`,
        email: newPaypalEmail,
        name: newPaypalAccountName,
        isVerified: true, // In real implementation, this would be false initially
        isDefault: paypalAccounts.length === 0, // First account becomes default
      }

      setPaypalAccounts((prev) => [...prev, newAccount])
      setSelectedPaypalAccount(newAccount.id)

      toast({
        title: "PayPal Account Connected",
        description: `Successfully connected ${newPaypalEmail} to your account.`,
      })

      // Reset form and close modal
      setNewPaypalEmail("")
      setNewPaypalPassword("")
      setNewPaypalFirstName("")
      setNewPaypalLastName("")
      setNewPaypalPhone("")
      setNewPaypalAccountName("")
      setIsPaypalModalOpen(false)
    } catch (error) {
      toast({
        title: "Connection Failed",
        description: "Failed to connect PayPal account. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsConnectingPaypal(false)
    }
  }

  const handleApproveLocation = async (locationId: number) => {
    const update: UpdateLocationApprovalRequest = {
        id: locationId,
        approval: true
    }
    try {
    updateLocationApproval(update)
      toast({
        title: "Location #" + locationId + "Approved",
        description: "Location has been successfully approved.",
      })
    } catch (error) {
      toast({
        title: "Approval Failed",
        description: "Failed to approve merchant. Please try again.",
      })
    }
  }

  const handleRejectLocation = async (locationId: number) => {
    const update: UpdateLocationApprovalRequest = {
        id: locationId,
        approval: false
    }
     try {
      updateLocationApproval(update)
      toast({
        title: "Location #" + locationId + "Rejected",
        description: "Location has been successfully rejected.",
      })
    } catch (error) {
      toast({
        title: "Approval Failed",
        description: "Failed to approve merchant. Please try again.",
      })
    }
  }

  const handleGenerateQRCodes = async () => {
    if (!eventStartDate || !eventEndDate || !eventStartTime || !eventEndTime || !sfluvPerCode || !numberOfCodes) {
      toast({
        title: "Missing Information",
        description: "Please fill in all required fields to generate QR codes.",
        variant: "destructive",
      })
      return
    }

    const sfluvAmount = Number.parseFloat(sfluvPerCode)
    const codeCount = Number.parseInt(numberOfCodes)

    if (sfluvAmount <= 0 || codeCount <= 0 || codeCount > 1000) {
      toast({
        title: "Invalid Values",
        description: "Please enter valid amounts. Maximum 1000 QR codes per event.",
        variant: "destructive",
      })
      return
    }

    if (eventStartDate >= eventEndDate) {
      toast({
        title: "Invalid Date Range",
        description: "Event end date must be after start date.",
        variant: "destructive",
      })
      return
    }

    setIsGeneratingCodes(true)
    try {
      // Simulate QR code generation
      await new Promise((resolve) => setTimeout(resolve, 2000))

      const codes = Array.from({ length: codeCount }, (_, index) => ({
        id: `qr-${Date.now()}-${index}`,
        eventId: `event-${Date.now()}`,
        codeNumber: index + 1,
        sfluvAmount,
        startDate: eventStartDate,
        endDate: eventEndDate,
        startTime: eventStartTime,
        endTime: eventEndTime,
        isRedeemed: false,
        qrData: `sfluv://redeem?event=${Date.now()}&code=${index + 1}&amount=${sfluvAmount}`,
      }))

      setGeneratedCodes(codes)

      toast({
        title: "QR Codes Generated",
        description: `Successfully generated ${codeCount} QR codes.`,
      })
    } catch (error) {
      toast({
        title: "Generation Failed",
        description: "Failed to generate QR codes. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsGeneratingCodes(false)
    }
  }

  const handleDownloadQRCodes = () => {
    if (generatedCodes.length === 0) {
      toast({
        title: "No QR Codes",
        description: "Please generate QR codes first before downloading.",
        variant: "destructive",
      })
      return
    }

    // Create CSV content
    const csvContent = [
      "Code Number,SFLUV Amount,QR Data,Start Date,End Date,Start Time,End Time",
      ...generatedCodes.map(
        (code) =>
          `${code.codeNumber},${code.sfluvAmount},"${code.qrData}","${code.startDate.toLocaleDateString()}","${code.endDate.toLocaleDateString()}","${code.startTime}","${code.endTime}"`,
      ),
    ].join("\n")

    // Create and download file
    const blob = new Blob([csvContent], { type: "text/csv" })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement("a")
    link.href = url
    link.download = `QR_Codes_${new Date().toISOString().split("T")[0]}.csv`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)

    toast({
      title: "Download Started",
      description: `QR codes have been downloaded as CSV.`,
    })
  }

  // Get available balance for conversion type
  const getAvailableBalance = () => {
    if (selectedWallet == null) return "No wallet selected"

    if (conversionType === "wrap") {
      return `$${selectedWalletBYUSDBalance.toLocaleString()} BYUSD`
    } else {
      return `${selectedWalletSFLUVBalance.toLocaleString()} SFLUV`
    }
  }

  // Get available balance for PayPal conversion
  const getPaypalAvailableBalance = () => {
    if (selectedWallet == null) return "No wallet selected"
    return `$${selectedWalletBYUSDBalance.toLocaleString()} BYUSD`
  }

  if(status === "loading") {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Admin Panel</h1>
          <p className="text-muted-foreground">Manage tokens and merchant approvals</p>
        </div>
      </div>

      <Tabs defaultValue="merchants" className="space-y-6">
        <TabsList className="grid w-full grid-cols-3">
          {/* <TabsTrigger value="tokens">Token Management</TabsTrigger> */}
          <TabsTrigger value="merchants" className="relative">
            Merchant Approvals
            {pendingLocations.length > 0 && (
              <Badge variant="destructive" className="ml-2 h-5 w-5 rounded-full p-1.5 text-xs">
                {pendingLocations.length}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="w9" className="relative">
            W9 Approvals
            {pendingW9Submissions.length > 0 && (
              <Badge variant="destructive" className="ml-2 h-5 w-5 rounded-full p-1.5 text-xs">
                {pendingW9Submissions.length}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="events" className="relative">
            Events
            {pendingLocations.length > 0 && (
              <Badge variant="destructive" className="ml-2 h-5 w-5 rounded-full p-1.5 text-xs">
                {pendingLocations.length}
              </Badge>
            )}
          </TabsTrigger>
          {/* <TabsTrigger value="qrcodes">QR Codes</TabsTrigger> */}
        </TabsList>

        <TabsContent value="tokens" className="space-y-6">
          {/* Global Wallet Selection */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Wallet className="h-5 w-5" />
                Select Wallet
              </CardTitle>
              <CardDescription>Choose which wallet to use for all operations</CardDescription>
            </CardHeader>
            <CardContent>
              <Select value={selectedWallet ? String(selectedWallet?.id) : ""} onValueChange={(id) => {
                const wallet = wallets.find((w) => String(w.id) === id)
                if (wallet) {
                setSelectedWallet(wallet)
                }
              }}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Choose a wallet to manage" />
                </SelectTrigger>
                <SelectContent>
                  {wallets.map((wallet) => (
                    <SelectItem key={wallet.name} value={String(wallet.id)}>
                      <div className="flex items-center gap-3">
                        <Wallet className="h-4 w-4" />
                        <div className="flex flex-col">
                          <span className="font-medium">{wallet.name}</span>
                          <span className="text-xs text-muted-foreground">{wallet.address}</span>
                        </div>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </CardContent>
          </Card>

          {/* Balance Overview */}
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">
                  {selectedWallet ? `${selectedWallet.name} BYUSD Balance` : "BYUSD Balance"}
                </CardTitle>
                <Coins className="h-4 w-4 text-blue-600" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  ${selectedWallet ? selectedWalletBYUSDBalance.toLocaleString() + " BYUSD" : "0"}
                </div>
                <p className="text-xs text-muted-foreground">
                  {selectedWallet ? "Available in selected wallet" : "Select a wallet to view balance"}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">
                  {selectedWallet ? `${selectedWallet.name} SFLUV Balance` : "SFLUV Balance"}
                </CardTitle>
                <Coins className="h-4 w-4 text-green-600" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {selectedWallet ? selectedWalletSFLUVBalance.toLocaleString() + " SFLUV": "0"}
                </div>
                <p className="text-xs text-muted-foreground">
                  {selectedWallet ? "Available in selected wallet" : "Select a wallet to view balance"}
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Token Conversion Interface */}
          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <ArrowUpDown className="h-5 w-5 text-green-600" />
                  Token Conversion
                </CardTitle>
                <CardDescription>Convert between BYUSD stablecoins and SFLUV tokens</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="conversion-type">Conversion Type</Label>
                  <Select value={conversionType} onValueChange={(value: "wrap" | "unwrap") => setConversionType(value)}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select conversion type" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="wrap">
                        <div className="flex items-center gap-2">
                          <ArrowUp className="h-4 w-4 text-green-600" />
                          Wrap
                        </div>
                      </SelectItem>
                      <SelectItem value="unwrap">
                        <div className="flex items-center gap-2">
                          <ArrowDown className="h-4 w-4 text-blue-600" />
                          Unwrap
                        </div>
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="conversion-amount">Amount to {conversionType === "wrap" ? "Wrap" : "Unwrap"}</Label>
                  <Input
                    id="conversion-amount"
                    type="text"
                    placeholder={`Enter ${conversionType === "wrap" ? "BYUSD" : "SFLUV"} amount`}
                    value={amount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    disabled={isProcessing}
                  />
                  <p className="text-sm text-muted-foreground">Available: {getAvailableBalance()}</p>
                </div>

                <Button
                  onClick={handleTokenConversion}
                  disabled={isProcessing || !amount || !selectedWallet}
                  className="w-full"
                >
                  {isProcessing ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      {conversionType === "wrap" ? "Wrapping..." : "Unwrapping..."}
                    </>
                  ) : (
                    <>
                      {conversionType === "wrap" ? (
                        <ArrowUp className="mr-2 h-4 w-4" />
                      ) : (
                        <ArrowDown className="mr-2 h-4 w-4" />
                      )}
                      {conversionType === "wrap" ? "Wrap to SFLUV" : "Unwrap to BYUSD"}
                    </>
                  )}
                </Button>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <CreditCard className="h-5 w-5 text-purple-600" />
                  PayPal Cash Conversion
                </CardTitle>
                <CardDescription>Convert BYUSD stablecoins to cash in your PayPal account</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="paypal-account">PayPal Account</Label>
                  <div className="flex gap-2">
                    <Select value={selectedPaypalAccount} onValueChange={setSelectedPaypalAccount}>
                      <SelectTrigger className="flex-1">
                        <SelectValue placeholder="Select PayPal account">
                          {selectedPaypalData && (
                            <div className="flex items-center gap-2 truncate">
                              <CreditCard className="h-4 w-4 flex-shrink-0" />
                              <span className="truncate">{selectedPaypalData.name}</span>
                            </div>
                          )}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {paypalAccounts.map((account) => (
                          <SelectItem key={account.id} value={account.id}>
                            <div className="flex items-center gap-2">
                              <CreditCard className="h-4 w-4" />
                              <div className="flex flex-col">
                                <span className="font-medium">{account.name}</span>
                                <span className="text-xs text-muted-foreground">{account.email}</span>
                              </div>
                              {account.isDefault && (
                                <Badge variant="secondary" className="ml-2 text-xs">
                                  Default
                                </Badge>
                              )}
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Dialog open={isPaypalModalOpen} onOpenChange={setIsPaypalModalOpen}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="icon">
                          <Plus className="h-4 w-4" />
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-[500px]">
                        <DialogHeader>
                          <DialogTitle>Connect PayPal Account</DialogTitle>
                          <DialogDescription>
                            Enter your PayPal credentials to connect your account for cash conversions.
                          </DialogDescription>
                        </DialogHeader>
                        <div className="grid gap-4 py-4 max-h-[60vh] overflow-y-auto">
                          <div className="grid grid-cols-2 gap-4">
                            <div className="space-y-2">
                              <Label htmlFor="paypal-first-name">
                                First Name <span className="text-red-500">*</span>
                              </Label>
                              <Input
                                id="paypal-first-name"
                                type="text"
                                placeholder="John"
                                value={newPaypalFirstName}
                                onChange={(e) => setNewPaypalFirstName(e.target.value)}
                                disabled={isConnectingPaypal}
                              />
                            </div>
                            <div className="space-y-2">
                              <Label htmlFor="paypal-last-name">
                                Last Name <span className="text-red-500">*</span>
                              </Label>
                              <Input
                                id="paypal-last-name"
                                type="text"
                                placeholder="Doe"
                                value={newPaypalLastName}
                                onChange={(e) => setNewPaypalLastName(e.target.value)}
                                disabled={isConnectingPaypal}
                              />
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-email">
                              PayPal Email Address <span className="text-red-500">*</span>
                            </Label>
                            <Input
                              id="paypal-email"
                              type="email"
                              placeholder="your-email@example.com"
                              value={newPaypalEmail}
                              onChange={(e) => setNewPaypalEmail(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-password">
                              PayPal Password <span className="text-red-500">*</span>
                            </Label>
                            <div className="relative">
                              <Input
                                id="paypal-password"
                                type={showPassword ? "text" : "password"}
                                placeholder="Enter your PayPal password"
                                value={newPaypalPassword}
                                onChange={(e) => setNewPaypalPassword(e.target.value)}
                                disabled={isConnectingPaypal}
                                className="pr-10"
                              />
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                                onClick={() => setShowPassword(!showPassword)}
                                disabled={isConnectingPaypal}
                              >
                                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                              </Button>
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-phone">Phone Number (Optional)</Label>
                            <Input
                              id="paypal-phone"
                              type="tel"
                              placeholder="+1 (555) 123-4567"
                              value={newPaypalPhone}
                              onChange={(e) => setNewPaypalPhone(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-account-name">
                              Account Display Name <span className="text-red-500">*</span>
                            </Label>
                            <Input
                              id="paypal-account-name"
                              type="text"
                              placeholder="My PayPal Account"
                              value={newPaypalAccountName}
                              onChange={(e) => setNewPaypalAccountName(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                            <p className="text-xs text-muted-foreground">
                              This name will be displayed in the account selection dropdown.
                            </p>
                          </div>
                          <div className="p-4 bg-blue-50 dark:bg-blue-950 rounded-lg">
                            <div className="flex items-start gap-2">
                              <CreditCard className="h-4 w-4 text-blue-600 mt-0.5 flex-shrink-0" />
                              <div className="text-xs text-blue-700 dark:text-blue-300">
                                <p className="font-medium mb-1">Secure Connection</p>
                                <p>
                                  Your PayPal credentials are encrypted and securely transmitted. We use PayPal's
                                  official API to verify and connect your account.
                                </p>
                              </div>
                            </div>
                          </div>
                        </div>
                        <DialogFooter>
                          <Button
                            variant="outline"
                            onClick={() => {
                              setIsPaypalModalOpen(false)
                              // Reset form when closing
                              setNewPaypalEmail("")
                              setNewPaypalPassword("")
                              setNewPaypalFirstName("")
                              setNewPaypalLastName("")
                              setNewPaypalPhone("")
                              setNewPaypalAccountName("")
                              setShowPassword(false)
                            }}
                            disabled={isConnectingPaypal}
                          >
                            Cancel
                          </Button>
                          <Button onClick={handleConnectPaypalAccount} disabled={isConnectingPaypal}>
                            {isConnectingPaypal ? (
                              <>
                                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                Connecting...
                              </>
                            ) : (
                              <>
                                <CreditCard className="mr-2 h-4 w-4" />
                                Connect Account
                              </>
                            )}
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="paypal-amount">Amount to Convert</Label>
                  <Input
                    id="paypal-amount"
                    type="text"
                    placeholder="0.00"
                    value={paypalAmount}
                    onChange={(e) => handlePaypalAmountChange(e.target.value)}
                    disabled={isConvertingToPaypal}
                  />
                  <p className="text-sm text-muted-foreground">Available: {getPaypalAvailableBalance()}</p>
                </div>

                <div className="p-3 bg-blue-50 dark:bg-blue-950 rounded-lg">
                  <p className="text-xs text-blue-700 dark:text-blue-300">
                    <strong>Note:</strong> Funds will be transferred to your selected PayPal account. Processing may
                    take 1-3 business days.
                  </p>
                </div>

                <Button
                  onClick={handlePaypalConversion}
                  disabled={isConvertingToPaypal || !paypalAmount || !selectedWallet || !selectedPaypalAccount}
                  variant="outline"
                  className="w-full bg-transparent border-purple-200 hover:bg-purple-50"
                >
                  {isConvertingToPaypal ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Converting to PayPal...
                    </>
                  ) : (
                    <>
                      <CreditCard className="mr-2 h-4 w-4" />
                      Convert to PayPal Cash
                    </>
                  )}
                </Button>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="merchants" className="space-y-6">
          <Card>
            <CardHeader className="pb-6">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-6 w-6" />
                Locations Pending Approval
              </CardTitle>
              <CardDescription className="text-base mt-2">Review and approve location applications</CardDescription>
              <div className="flex items-center gap-2 mt-3">
                <Badge variant="destructive" className="text-sm px-3 py-1">
                  {pendingLocations.length} Pending
                </Badge>
                <span className="text-sm text-muted-foreground">applications awaiting review</span>
              </div>
            </CardHeader>
            <CardContent>
              {pendingLocations.length === 0 ? (
                <div className="text-center py-8">
                  <Building2 className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Pending Locations</h3>
                  <p className="text-muted-foreground">All location applications have been processed.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {pendingLocations.map((location) => (
                    <Card key={location.id} className="border-l-4 border-l-yellow-500">
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex items-start gap-4 flex-1">
                            <Avatar className="h-12 w-12">
                              <AvatarImage src={location.image_url || "/placeholder.svg"} alt={location.name} />
                              <AvatarFallback>
                                {location.name
                                  .split(" ")
                                  .map((n) => n[0])
                                  .join("")}
                              </AvatarFallback>
                            </Avatar>
                            <div className="flex-1 space-y-2">
                              <div>
                                <h4 className="font-semibold">{location.name}</h4>
                                <p className="text-sm text-muted-foreground">{location.type}</p>
                              </div>
                              <div className="grid gap-1 text-sm">
                                <div className="flex items-center gap-2">
                                  <Mail className="h-3 w-3" />
                                  <span>{location.email}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  <Phone className="h-3 w-3" />
                                  <span>{location.phone}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  <MapPin className="h-3 w-3" />
                                  <span>
                                    {location.street}, {location.city}, {location.state}{" "}
                                    {location.zip}
                                  </span>
                                </div>
                              </div>
                            </div>
                          </div>
                          <div className="flex gap-2">
                            <Button
                              size="sm"
                              onClick={() => handleApproveLocation(location.id)}
                              disabled={!pendingLocations.includes(location)}
                            >
                              {!pendingLocations.includes(location) ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Check className="h-4 w-4" />
                              )}
                              Approve
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => handleRejectLocation(location.id)}
                              disabled={!pendingLocations.includes(location)}
                            >
                              {!pendingLocations.includes(location) ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <X className="h-4 w-4" />
                              )}
                              Reject
                            </Button>
                            <Button
                              size="sm"
                              onClick={() => {
                                setselectedLocationForReview(location)
                                setisLocationReviewModalOpen(true)
                              }}
                            >
                              Review Application
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="w9" className="space-y-6">
          <Card>
            <CardHeader className="pb-6">
              <CardTitle className="flex items-center gap-2 text-xl">
                <FileCheck className="h-6 w-6" />
                W9 Submissions Pending Approval
              </CardTitle>
              <CardDescription className="text-base mt-2">Review and approve W9 submissions</CardDescription>
              <div className="flex items-center gap-2 mt-3">
                <Badge variant="destructive" className="text-sm px-3 py-1">
                  {pendingW9Submissions.length} Pending
                </Badge>
                <span className="text-sm text-muted-foreground">submissions awaiting review</span>
              </div>
            </CardHeader>
            <CardContent>
              {w9Loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin" />
                </div>
              ) : pendingW9Submissions.length === 0 ? (
                <div className="text-center py-8">
                  <FileCheck className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Pending W9 Submissions</h3>
                  <p className="text-muted-foreground">All W9 submissions have been processed.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {pendingW9Submissions.map((submission) => (
                    <Card key={submission.id} className="border-l-4 border-l-yellow-500">
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex-1 space-y-2">
                            <div>
                              <h4 className="font-semibold">Wallet</h4>
                              <p className="text-sm text-muted-foreground break-all">{submission.wallet_address}</p>
                            </div>
                            <div className="grid gap-1 text-sm">
                              <div className="flex items-center gap-2">
                                <Mail className="h-3 w-3" />
                                <span>{submission.email}</span>
                              </div>
                              <div className="flex items-center gap-2">
                                <CalendarIcon className="h-3 w-3" />
                                <span>Year {submission.year}</span>
                              </div>
                            </div>
                          </div>
                          <div className="flex gap-2">
                            <Button size="sm" onClick={() => handleApproveW9(submission.id)}>
                              <Check className="h-4 w-4" />
                              Approve
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events" className="space-y-6">
          <AddEventModal
            open={eventsModalOpen}
            onOpenChange={toggleNewEventModal}
            handleAddEvent={handleAddEvent}
            addEventError={eventsError}
          />
          <EventModal
            event={eventDetailsEvent}
            open={eventDetailModalOpen}
            onOpenChange={toggleEventDetailModal}
            handleDeleteEvent={handleDeleteEvent}
            deleteEventError={deleteEventError}
          />
          <DrainFaucetModal
            open={drainFaucetModalOpen}
            onOpenChange={toggleDrainFaucetModal}
            handleDrainFaucet={handleDrainFaucet}
            drainFaucetError={drainFaucetError}
          />
          <Card>
            <CardHeader className="pb-6 grid grid-cols-[2fr,1fr]">
              <div>
                <CardTitle className="flex items-center gap-2 text-xl">
                  <CalendarIcon className="h-6 w-6" />
                  Volunteer Events
                </CardTitle>
                <CardDescription className="text-base mt-2">Create and Manage Volunteer Events</CardDescription>
                <div className="flex items-center gap-2 mt-3">
                  <Badge className="text-sm px-3 py-1 cursor-pointer" onClick={toggleDrainFaucetModal}>
                    {faucetBalance} SFLuv
                  </Badge>
                  <span className="text-sm text-muted-foreground">in faucet</span>
                </div>
              </div>
              <div className="text-right">
                <Button onClick={toggleNewEventModal}>
                  + New Event
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {events.length === 0 ? (
                <div className="text-center py-8">
                  <Leaf className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No {eventsExpired ? "" : "Active"} Events</h3>
                  <p className="text-muted-foreground">Create a new event to see it here.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {events.map((event: Event) => (
                    <EventCard
                      key={event.id}
                      event={event}
                      toggleEventModal={toggleEventDetailModal}
                      setEventModalEvent={setEventDetailsEvent}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="qrcodes" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <QrCode className="h-5 w-5" />
                Generate Event QR Codes
              </CardTitle>
              <CardDescription>
                Create QR codes for volunteer events that can be redeemed for SFLuv tokens
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Event Information */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold">Event Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>Event Start Date</Label>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button
                          variant="outline"
                          className={cn(
                            "w-full justify-start text-left font-normal",
                            !eventStartDate && "text-muted-foreground",
                          )}
                          disabled={isGeneratingCodes}
                        >
                          <CalendarIcon className="mr-2 h-4 w-4" />
                          {eventStartDate ? eventStartDate.toLocaleDateString() : "Select start date"}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-auto p-0">
                        <Calendar
                          mode="single"
                          selected={eventStartDate}
                          onSelect={setEventStartDate}
                          initialFocus
                          className="border-none shadow-md"
                        />
                      </PopoverContent>
                    </Popover>
                  </div>

                  <div className="space-y-2">
                    <Label>Event End Date</Label>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button
                          variant="outline"
                          className={cn(
                            "w-full justify-start text-left font-normal",
                            !eventEndDate && "text-muted-foreground",
                          )}
                          disabled={isGeneratingCodes}
                        >
                          <CalendarIcon className="mr-2 h-4 w-4" />
                          {eventEndDate ? eventEndDate.toLocaleDateString() : "Select end date"}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-auto p-0">
                        <Calendar
                          mode="single"
                          selected={eventEndDate}
                          onSelect={setEventEndDate}
                          initialFocus
                          className="border-none shadow-md"
                        />
                      </PopoverContent>
                    </Popover>
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="start-time">Start Time</Label>
                    <Input
                      id="start-time"
                      type="time"
                      value={eventStartTime}
                      onChange={(e) => setEventStartTime(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="end-time">End Time</Label>
                    <Input
                      id="end-time"
                      type="time"
                      value={eventEndTime}
                      onChange={(e) => setEventEndTime(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                  </div>
                </div>
              </div>

              {/* QR Code Configuration */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold">QR Code Configuration</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="sfluv-per-code">SFLuv per QR Code</Label>
                    <Input
                      id="sfluv-per-code"
                      type="text"
                      placeholder="50"
                      value={sfluvPerCode}
                      onChange={(e) => {
                        const formatted = formatAmount(e.target.value)
                        setSfluvPerCode(formatted)
                      }}
                      disabled={isGeneratingCodes}
                    />
                    <p className="text-xs text-muted-foreground">Amount of SFLuv each volunteer will receive</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="number-of-codes">Number of QR Codes</Label>
                    <Input
                      id="number-of-codes"
                      type="number"
                      placeholder="100"
                      min="1"
                      max="1000"
                      value={numberOfCodes}
                      onChange={(e) => setNumberOfCodes(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                    <p className="text-xs text-muted-foreground">Maximum 1000 codes per event</p>
                  </div>
                </div>

                <div className="p-4 bg-blue-50 dark:bg-blue-950 rounded-lg">
                  <div className="flex items-start gap-2">
                    <QrCode className="h-4 w-4 text-blue-600 mt-0.5 flex-shrink-0" />
                    <div className="text-xs text-blue-700 dark:text-blue-300">
                      <p className="font-medium mb-1">QR Code Usage</p>
                      <p>
                        Each QR code can only be redeemed once during the event period. Volunteers scan the code to
                        receive their SFLuv directly to their wallet.
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              {/* Generation Actions */}
              <div className="flex gap-4">
                <Button
                  onClick={handleGenerateQRCodes}
                  disabled={isGeneratingCodes || !eventStartDate || !eventEndDate}
                  className="flex-1"
                >
                  {isGeneratingCodes ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Generating QR Codes...
                    </>
                  ) : (
                    <>
                      <QrCode className="mr-2 h-4 w-4" />
                      Generate QR Codes
                    </>
                  )}
                </Button>

                {generatedCodes.length > 0 && (
                  <Button onClick={handleDownloadQRCodes} variant="outline" disabled={isGeneratingCodes}>
                    <Download className="mr-2 h-4 w-4" />
                    Download CSV
                  </Button>
                )}
              </div>

              {/* Generated Codes Preview */}
              {generatedCodes.length > 0 && (
                <div className="space-y-4">
                  <h3 className="text-lg font-semibold">Generated QR Codes</h3>

                  <div className="p-4 bg-green-50 dark:bg-green-950 rounded-lg">
                    <div className="flex items-center gap-2 mb-2">
                      <Check className="h-4 w-4 text-green-600" />
                      <span className="font-medium text-green-700 dark:text-green-300">Successfully Generated</span>
                    </div>
                    <div className="text-sm text-green-700 dark:text-green-300 space-y-1">
                      <p>
                        <strong>QR Codes:</strong> {generatedCodes.length}
                      </p>
                      <p>
                        <strong>SFLuv per Code:</strong> {sfluvPerCode}
                      </p>
                      <p>
                        <strong>Total SFLuv:</strong>{" "}
                        {(Number.parseFloat(sfluvPerCode) * generatedCodes.length).toLocaleString()}
                      </p>
                      <p>
                        <strong>Event Period:</strong> {eventStartDate?.toLocaleDateString()} {eventStartTime} -{" "}
                        {eventEndDate?.toLocaleDateString()} {eventEndTime}
                      </p>
                    </div>
                  </div>

                  <div className="border rounded-lg p-4 max-h-60 overflow-y-auto">
                    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2 text-xs">
                      {generatedCodes.slice(0, 20).map((code) => (
                        <div key={code.id} className="p-2 bg-muted rounded text-center">
                          <div className="font-mono">Code #{code.codeNumber}</div>
                          <div className="text-muted-foreground">{code.sfluvAmount} SFLuv</div>
                        </div>
                      ))}
                      {generatedCodes.length > 20 && (
                        <div className="p-2 bg-muted rounded text-center text-muted-foreground">
                          +{generatedCodes.length - 20} more codes
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Location Review Modal */}
      <Dialog open={isLocationReviewModalOpen} onOpenChange={setisLocationReviewModalOpen}>
        <DialogContent className="sm:max-w-[900px] max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="text-xl font-bold">
              Location Application Review - {selectedLocationForReview?.name}
            </DialogTitle>
            <DialogDescription>Review the complete merchant application details below</DialogDescription>
          </DialogHeader>

          {selectedLocationForReview && (
            <div className="space-y-6 py-4">
              {/* Business Information Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Name</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.name}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Type</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.type}
                    </p>
                  </div>

                  <div className="md:col-span-2">
                    <Label className="text-sm font-medium text-muted-foreground">Business Description</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.description}
                    </p>
                  </div>

                </div>
              </div>

              {/* Contact Information Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Contact Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Primary Contact Name</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">{selectedLocationForReview.contact_firstname + " " + selectedLocationForReview.contact_lastname}</p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Admin Email Address</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.admin_email}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Admin Phone Number</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.admin_phone}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Website</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                     {selectedLocationForReview.website}
                    </p>
                  </div>
                </div>
              </div>

              {/* Business Address Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Address</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="md:col-span-2">
                    <Label className="text-sm font-medium text-muted-foreground">Street Address</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.street}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">City</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.city}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">State</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.state}
                    </p>
                  </div>
                </div>
              </div>

              {/* SFLuv Integration Section */}
              {/*
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">SFLuv Integration Questions</h3>

                <div className="space-y-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      Why do you want to join the SFLuv network?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-3 bg-muted rounded">
                      We believe in supporting our local community and want to be part of a network that promotes local
                      businesses. SFLuv aligns with our values of community engagement and supporting the local economy.
                      We see this as an opportunity to connect with more local customers and contribute to the growth of
                      San Francisco's small business ecosystem.
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      How do you plan to integrate SFLuv tokens into your business operations?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-3 bg-muted rounded">
                      We plan to accept SFLuv tokens as payment for all our products and services. We will also offer
                      special discounts and promotions for customers who pay with SFLuv tokens to encourage adoption.
                      Additionally, we're interested in participating in community events and offering rewards to loyal
                      customers through the SFLuv platform.
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      What percentage of transactions do you expect to process with SFLuv tokens?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">15-25% within the first year</p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      Do you have experience with cryptocurrency or digital payment systems?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      Yes, we currently accept various digital payment methods including mobile payments and have basic
                      knowledge of cryptocurrency systems.
                    </p>
                  </div>
                </div>
              </div>
              */}

              {/* Business Verification Section */}
              {/*<div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Verification</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business License Number</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      BL-SF-{Math.floor(Math.random() * 100000)}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Tax ID (EIN)</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      **-***{Math.floor(Math.random() * 10000)}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Insurance</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      ✅ General Liability Insurance Active
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Application Date</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {new Date().toLocaleDateString()}
                    </p>
                  </div>
                </div>
              </div>
            */}
            </div>
          )}

          <DialogFooter className="flex gap-2">
            <Button variant="outline" onClick={() => setisLocationReviewModalOpen(false)}>
              Close
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (selectedLocationForReview) {
                  handleRejectLocation(selectedLocationForReview.id)
                  setisLocationReviewModalOpen(false)
                }
              }}
            >
              Reject Application
            </Button>
            <Button
              onClick={() => {
                if (selectedLocationForReview) {
                  handleApproveLocation(selectedLocationForReview.id)
                  setisLocationReviewModalOpen(false)
                }
              }}
            >
              Approve Location
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
