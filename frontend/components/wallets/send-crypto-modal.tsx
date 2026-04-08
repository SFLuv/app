"use client";

import type React from "react";

import { useEffect, useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent } from "@/components/ui/card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Send,
  AlertTriangle,
  CheckCircle,
  X,
  Copy,
  ArrowLeft,
  Camera,
  ImageUp,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useApp } from "@/context/AppProvider";
import { AppWallet } from "@/lib/wallets/wallets";
import { SFLUV_DECIMALS, SYMBOL } from "@/lib/constants";
import { Address, Hash } from "viem";
import { useContacts } from "@/context/ContactsProvider";
import ContactOrAddressInput from "../contacts/contact-or-address-input";
import type { W9Submission } from "@/types/w9";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  extractEthereumAddressFromPayload,
  extractMerchantSendFromPayload,
  extractRedeemParamsFromPayload,
} from "@/lib/qr/payload";
import jsQR from "jsqr";

type SendFlowMode = "manual" | "scan";
type SendStep =
  | "form"
  | "confirm"
  | "sending"
  | "success"
  | "tip_prompt"
  | "tip_sending"
  | "tip_success"
  | "error";

type WalletLookupResponse = {
  found?: boolean;
  user_id?: string;
  is_merchant?: boolean;
  merchant_name?: string;
  wallet_name?: string;
  address?: string;
  matched_primary_wallet?: boolean;
  matched_payment_wallet?: boolean;
  pay_to_address?: string;
  tip_to_address?: string;
};

type TipPromptState = {
  merchantName: string;
  tipToAddress: string;
  amount: string;
};

interface SendCryptoModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wallet: AppWallet;
  balance: number;
  defaultFlow?: SendFlowMode;
  defaultRecipient?: string;
  defaultTipTo?: string;
}

export function SendCryptoModal({
  open,
  onOpenChange,
  wallet,
  balance,
  defaultFlow = "manual",
  defaultRecipient,
  defaultTipTo,
}: SendCryptoModalProps) {
  const [step, setStep] = useState<SendStep>("form");
  const [flowMode, setFlowMode] = useState<SendFlowMode>(defaultFlow);
  const [hash, setHash] = useState<Hash | null>(null);
  const [copied, setCopied] = useState<boolean>(false);
  const [w9Email, setW9Email] = useState<string | null>(null);
  const [w9Reason, setW9Reason] = useState<"w9_required" | "w9_pending" | null>(
    null,
  );
  const [w9Year, setW9Year] = useState<number | null>(null);
  const [w9EmailInput, setW9EmailInput] = useState<string>("");
  const [w9Submitting, setW9Submitting] = useState<boolean>(false);
  const [formData, setFormData] = useState({
    recipient: defaultRecipient ?? "",
    amount: "",
    memo: "",
  });
  const [error, setError] = useState("");
  const [scanError, setScanError] = useState<string>("");
  const [scannerRunning, setScannerRunning] = useState<boolean>(false);
  const [scannerSupported, setScannerSupported] = useState<boolean>(true);
  const [photoScanLoading, setPhotoScanLoading] = useState<boolean>(false);
  const [scanInstruction, setScanInstruction] = useState<string>(
    "Point your camera at a QR code.",
  );
  const [showScanMoreOptions, setShowScanMoreOptions] =
    useState<boolean>(false);
  const [processingDetectedQr, setProcessingDetectedQr] =
    useState<boolean>(false);
  const [recipientMerchantName, setRecipientMerchantName] =
    useState<string>("");
  const [recipientIsMerchant, setRecipientIsMerchant] =
    useState<boolean>(false);
  const [recipientMatchedPaymentWallet, setRecipientMatchedPaymentWallet] =
    useState<boolean>(false);
  const [recipientTipToAddress, setRecipientTipToAddress] =
    useState<string>("");
  const [linkProvidedTipTo, setLinkProvidedTipTo] = useState<string>(
    defaultTipTo ?? "",
  );
  const [linkProvidedMerchantName, setLinkProvidedMerchantName] =
    useState<string>("");
  const [tipPrompt, setTipPrompt] = useState<TipPromptState | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const scanIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const scanLockedRef = useRef<boolean>(false);
  const isDetectingRef = useRef<boolean>(false);
  const router = useRouter();
  const pathname = usePathname();
  const pageSearchParams = useSearchParams();
  const { toast } = useToast();
  const { contacts } = useContacts();
  const { user, authFetch } = useApp();

  const isValidEmail = (email: string) =>
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);

  const toAmountWei = (amountValue: string) =>
    BigInt(Number(amountValue) * 10 ** SFLUV_DECIMALS);

  const resolveTipPrompt = (): TipPromptState | null => {
    const normalizedRecipient = normalizedRecipientAddress(formData.recipient);
    if (!normalizedRecipient) return null;

    // Prefer the tipTo provided by a scanned/pasted payment link or by the
    // /redirect handoff: those are an explicit instruction from the merchant
    // and don't depend on the recipient being registered in our DB. Fall back
    // to the merchant-lookup result for users entering an address manually.
    let tipSource: string | null = null;
    let merchantNameSource = "";

    const normalizedLinkTip = normalizedRecipientAddress(linkProvidedTipTo);
    if (normalizedLinkTip) {
      tipSource = normalizedLinkTip;
      merchantNameSource =
        linkProvidedMerchantName || recipientMerchantName || "";
    } else if (recipientIsMerchant && recipientMatchedPaymentWallet) {
      const normalizedLookupTip = normalizedRecipientAddress(
        recipientTipToAddress,
      );
      if (normalizedLookupTip) {
        tipSource = normalizedLookupTip;
        merchantNameSource = recipientMerchantName;
      }
    }

    if (!tipSource) return null;
    if (tipSource.toLowerCase() === normalizedRecipient.toLowerCase()) {
      return null;
    }

    return {
      merchantName: merchantNameSource || "this merchant",
      tipToAddress: tipSource,
      amount: "",
    };
  };

  const saveTransactionMemo = async (txHash: string) => {
    const memo = formData.memo.trim();
    if (!memo) return;

    try {
      const res = await authFetch("/transactions/memo", {
        method: "POST",
        body: JSON.stringify({
          tx_hash: txHash,
          memo,
        }),
      });
      if (!res.ok) {
        throw new Error("Failed to save transaction memo.");
      }
    } catch (memoError) {
      console.error(memoError);
      toast({
        title: "Memo Not Saved",
        description: "The transfer was sent, but the memo could not be saved.",
        variant: "destructive",
      });
    }
  };

  const stopScanner = () => {
    if (scanIntervalRef.current) {
      clearInterval(scanIntervalRef.current);
      scanIntervalRef.current = null;
    }
    if (streamRef.current) {
      for (const track of streamRef.current.getTracks()) {
        track.stop();
      }
      streamRef.current = null;
    }
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    setScannerRunning(false);
    isDetectingRef.current = false;
  };

  const processScannedPayload = async (rawValue: string) => {
    const value = rawValue.trim();
    if (!value || scanLockedRef.current) return;
    scanLockedRef.current = true;
    setProcessingDetectedQr(true);
    setScanError("");
    setError("");

    try {
      const redeemParams = extractRedeemParamsFromPayload(value);
      if (redeemParams) {
        const nextParams = new URLSearchParams(redeemParams.toString());
        nextParams.set("webWallet", "1");
        nextParams.set("source", "wallet");
        const currentSearch = pageSearchParams.toString();
        const returnTo = currentSearch
          ? `${pathname}?${currentSearch}`
          : pathname;
        nextParams.set("returnTo", returnTo);
        stopScanner();
        toast({
          title: "Redeem Code Detected",
          description: "Redirecting to redeem flow...",
        });
        handleClose();
        router.push(`/faucet/redeem?${nextParams.toString()}`);
        return;
      }

      const merchantSend = extractMerchantSendFromPayload(value);
      if (merchantSend) {
        setFormData((prev) => ({ ...prev, recipient: merchantSend.to }));
        setLinkProvidedTipTo(merchantSend.tipTo || "");
        setLinkProvidedMerchantName("");
        setScanInstruction("Payment link found. Opening payment details...");
        stopScanner();
        setStep("confirm");
        scanLockedRef.current = false;
        return;
      }

      const recipientAddress = extractEthereumAddressFromPayload(value);
      if (!recipientAddress) {
        setScanError(
          "Unsupported QR code. Scan a wallet address, CitizenWallet link, or faucet redeem code.",
        );
        scanLockedRef.current = false;
        return;
      }

      setFormData((prev) => ({ ...prev, recipient: recipientAddress }));
      setLinkProvidedTipTo("");
      setLinkProvidedMerchantName("");
      setScanInstruction("Address found. Opening payment details...");
      stopScanner();
      setStep("confirm");
      scanLockedRef.current = false;
    } finally {
      setProcessingDetectedQr(false);
    }
  };

  const detectFromSource = async (source: ImageBitmapSource) => {
    const barcodeDetector = (window as any)?.BarcodeDetector;
    if (barcodeDetector) {
      const detector = new barcodeDetector({ formats: ["qr_code"] });
      const codes = await detector.detect(source);
      if (codes && codes.length > 0 && codes[0]?.rawValue) {
        await processScannedPayload(codes[0].rawValue);
        return;
      }
    }

    const width =
      source instanceof HTMLVideoElement
        ? source.videoWidth || source.clientWidth
        : ((source as { width?: number }).width ?? 0);
    const height =
      source instanceof HTMLVideoElement
        ? source.videoHeight || source.clientHeight
        : ((source as { height?: number }).height ?? 0);

    if (!width || !height) {
      throw new Error("No QR code detected. Try again.");
    }

    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext("2d", { willReadFrequently: true });
    if (!ctx) {
      throw new Error("No QR code detected. Try again.");
    }

    ctx.drawImage(source as CanvasImageSource, 0, 0, width, height);
    const imageData = ctx.getImageData(0, 0, width, height);
    const decoded = jsQR(imageData.data, width, height, {
      inversionAttempts: "attemptBoth",
    });
    if (!decoded?.data) {
      throw new Error("No QR code detected. Try again.");
    }

    await processScannedPayload(decoded.data);
  };

  const startScanner = async () => {
    if (scannerRunning || streamRef.current) return;
    setScanError("");
    setScannerSupported(true);
    setScanInstruction("Point your camera at a QR code.");
    scanLockedRef.current = false;

    if (!navigator?.mediaDevices?.getUserMedia) {
      setScannerSupported(false);
      setScanError(
        "Camera access is not supported in this browser. Switch to manual flow.",
      );
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: { ideal: "environment" },
        },
      });

      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }

      setScannerRunning(true);
      scanIntervalRef.current = setInterval(async () => {
        if (
          !videoRef.current ||
          isDetectingRef.current ||
          scanLockedRef.current
        )
          return;
        isDetectingRef.current = true;
        try {
          await detectFromSource(videoRef.current);
        } catch {
          // Keep scanning until a valid payload is detected.
        } finally {
          isDetectingRef.current = false;
        }
      }, 600);
    } catch {
      setScanError("Camera permission denied or unavailable.");
      setScannerSupported(false);
      stopScanner();
    }
  };

  const handlePhotoScan = async (file: File) => {
    setPhotoScanLoading(true);
    setScanError("");
    try {
      const bitmap = await createImageBitmap(file);
      try {
        await detectFromSource(bitmap);
      } finally {
        bitmap.close();
      }
    } catch (err) {
      const message =
        err instanceof Error
          ? err.message
          : "Unable to read QR from selected photo.";
      setScanError(message);
    } finally {
      setPhotoScanLoading(false);
    }
  };

  const switchFlow = (nextFlow: SendFlowMode) => {
    if (nextFlow === flowMode) return;
    setFlowMode(nextFlow);
    setError("");
    setScanError("");
    setProcessingDetectedQr(false);
    setShowScanMoreOptions(false);
    if (nextFlow === "manual") {
      stopScanner();
    } else {
      setFormData((prev) => ({ ...prev, recipient: "" }));
    }
  };

  const normalizedRecipientAddress = (rawValue: string): string | null => {
    return extractEthereumAddressFromPayload(rawValue);
  };

  const openPhotoPicker = () => {
    fileInputRef.current?.click();
  };

  const handlePhotoInputChange = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];
    if (!file) return;
    await handlePhotoScan(file);
    event.target.value = "";
  };

  const executeSend = async ({
    recipient,
    amount,
    memo,
    onSuccess,
    successToast,
  }: {
    recipient: string;
    amount: string;
    memo?: string;
    onSuccess: (receiptHash: Hash) => void;
    successToast: string;
  }) => {
    try {
      const amountWei = toAmountWei(amount);
      const receipt = await wallet.send(amountWei, recipient as Address);

      if (!receipt) {
        setStep("error");
        setError("Error creating transaction. Please try again.");
        return;
      }

      if (receipt.hash) {
        if (memo?.trim()) {
          await saveTransactionMemo(receipt.hash);
        }
        setHash(receipt.hash as Hash);
        onSuccess(receipt.hash as Hash);
        toast({
          title: "Transaction Sent",
          description: successToast,
        });
        return;
      }

      setStep("error");
      setError("Transaction failed. Please try again.");
    } catch {
      setStep("error");
      setError("Transaction failed. Please try again.");
    }
  };

  const findPendingSubmissionId = async (
    walletAddress: string,
    year: number,
  ): Promise<number | null> => {
    const res = await authFetch("/admin/w9/pending");
    if (res.status !== 200) {
      throw new Error("Unable to fetch pending W9 submissions.");
    }
    const data = await res.json();
    const submissions: W9Submission[] = Array.isArray(data?.submissions)
      ? data.submissions
      : [];
    const normalizedWallet = walletAddress.toLowerCase();
    const matches = submissions.filter((submission) => {
      return (
        submission.pending_approval &&
        submission.wallet_address.toLowerCase() === normalizedWallet &&
        submission.year === year
      );
    });
    if (matches.length === 0) return null;
    matches.sort((a, b) => b.id - a.id);
    return matches[0].id;
  };

  const handleApproveAndSend = async () => {
    if (!user?.isAdmin) {
      setError("Only admins can approve W9 submissions.");
      return;
    }

    const email = w9EmailInput.trim();
    if (!email) {
      setError(
        "Recipient email is required to approve W9. Enter an email to continue.",
      );
      return;
    }
    if (!isValidEmail(email)) {
      setError("Please enter a valid recipient email.");
      return;
    }

    setW9Submitting(true);
    try {
      const year = w9Year ?? new Date().getUTCFullYear();
      let submissionId: number | null = null;
      let alreadyApproved = false;

      const submitRes = await authFetch("/w9/submit", {
        method: "POST",
        body: JSON.stringify({
          wallet_address: formData.recipient,
          email,
          year,
        }),
      });

      if (submitRes.status === 201) {
        const data = await submitRes.json();
        submissionId = data?.submission?.id ?? null;
      } else if (submitRes.status === 409) {
        const data = await submitRes.json().catch(() => null);
        const submitError = data?.error;
        if (submitError === "w9_approved") {
          alreadyApproved = true;
        } else if (submitError === "w9_pending") {
          submissionId = await findPendingSubmissionId(
            formData.recipient,
            year,
          );
          if (!submissionId) {
            throw new Error("Pending W9 submission not found for this wallet.");
          }
        } else {
          throw new Error("Unable to submit W9 for approval.");
        }
      } else {
        throw new Error("Unable to submit W9 for approval.");
      }

      if (!alreadyApproved) {
        if (!submissionId) {
          submissionId = await findPendingSubmissionId(
            formData.recipient,
            year,
          );
        }
        if (!submissionId) {
          throw new Error(
            "W9 submission could not be identified for approval.",
          );
        }

        const approveRes = await authFetch("/admin/w9/approve", {
          method: "PUT",
          body: JSON.stringify({ id: submissionId }),
        });
        if (approveRes.status === 409) {
          const approveData = await approveRes.json().catch(() => null);
          if (approveData?.error !== "w9_not_pending") {
            throw new Error("Unable to approve W9 submission.");
          }
        } else if (approveRes.status !== 200) {
          throw new Error("Unable to approve W9 submission.");
        }
      }

      toast({
        title: "W9 Approved",
        description: "Recipient W9 is approved. Continuing transfer.",
      });

      setW9Reason(null);
      setW9Year(null);
      setW9Email(email);
      setError("");
      setStep("sending");
      const tipPromptState = resolveTipPrompt();
      await executeSend({
        recipient: formData.recipient,
        amount: formData.amount,
        memo: formData.memo,
        successToast: `Successfully sent ${formData.amount} ${SYMBOL} to ${formData.recipient.slice(0, 6)}...${formData.recipient.slice(-4)}`,
        onSuccess: () => {
          setTipPrompt(tipPromptState);
          setStep("success");
        },
      });
    } catch (err) {
      setStep("error");
      setError(
        err instanceof Error
          ? err.message
          : "Failed to approve W9. Please try again.",
      );
    } finally {
      setW9Submitting(false);
    }
  };

  const copyHash = async () => {
    try {
      if (!hash) throw new Error("no hash to copy");
      await navigator.clipboard.writeText(hash);
      setCopied(true);
      toast({
        title: "Hash Copied",
        description: "Tx hash has been copied to clipboard",
      });
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      toast({
        title: "Copy Failed",
        description: "Failed to copy hash to clipboard",
        variant: "destructive",
      });
    }
  };

  useEffect(() => {
    return () => {
      stopScanner();
    };
  }, []);

  useEffect(() => {
    if (!open) {
      stopScanner();
      return;
    }

    setFlowMode(defaultFlow);
    setScanInstruction("Point your camera at a QR code.");
    setScanError("");
    setProcessingDetectedQr(false);
    setShowScanMoreOptions(false);
    scanLockedRef.current = false;
    if (defaultRecipient) {
      setFormData((prev) => ({ ...prev, recipient: defaultRecipient }));
    }
    setLinkProvidedTipTo(defaultTipTo ?? "");
    setLinkProvidedMerchantName("");

    // When the modal is opened with a recipient already prefilled in the
    // scan flow (e.g. arriving from /redirect after scanning a merchant QR
    // with the system camera), skip the camera entirely and land directly
    // on the scan-style confirm screen — the camera step has nothing left
    // to do, and the confirm view has the better amount-entry UX.
    if (defaultFlow === "scan" && defaultRecipient) {
      stopScanner();
      setStep("confirm");
    }
  }, [open, defaultFlow, defaultRecipient, defaultTipTo]);

  useEffect(() => {
    if (!open || step !== "form") return;
    if (flowMode !== "scan") {
      stopScanner();
      return;
    }
    if (formData.recipient) {
      stopScanner();
      return;
    }
    startScanner();
  }, [flowMode, formData.recipient, open, step]);

  useEffect(() => {
    if (!open) return;

    const recipient = normalizedRecipientAddress(formData.recipient);
    if (!recipient) {
      setRecipientMerchantName("");
      setRecipientIsMerchant(false);
      setRecipientMatchedPaymentWallet(false);
      setRecipientTipToAddress("");
      return;
    }

    let cancelled = false;
    const lookupRecipient = async () => {
      try {
        const res = await authFetch(
          `/wallets/lookup/${encodeURIComponent(recipient)}`,
        );
        if (!res.ok) {
          if (!cancelled) {
            setRecipientMerchantName("");
            setRecipientIsMerchant(false);
            setRecipientMatchedPaymentWallet(false);
            setRecipientTipToAddress("");
          }
          return;
        }

        const data = (await res.json()) as WalletLookupResponse;
        if (cancelled) return;

        if (data.found && data.is_merchant) {
          const merchantName = (
            data.merchant_name ||
            data.wallet_name ||
            ""
          ).trim();
          setRecipientMerchantName(merchantName || "Merchant");
          setRecipientIsMerchant(true);
          setRecipientMatchedPaymentWallet(
            data.matched_payment_wallet === true ||
              data.matched_primary_wallet === true,
          );
          setRecipientTipToAddress((data.tip_to_address || "").trim());
          return;
        }

        setRecipientMerchantName("");
        setRecipientIsMerchant(false);
        setRecipientMatchedPaymentWallet(false);
        setRecipientTipToAddress("");
      } catch {
        if (cancelled) return;
        setRecipientMerchantName("");
        setRecipientIsMerchant(false);
        setRecipientMatchedPaymentWallet(false);
        setRecipientTipToAddress("");
      }
    };

    void lookupRecipient();

    return () => {
      cancelled = true;
    };
  }, [authFetch, formData.recipient, open]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    // Basic validation
    if (!formData.recipient || !formData.amount) {
      setError("Please fill in all required fields");
      return;
    }

    if (Number.parseFloat(formData.amount) <= 0) {
      setError("Amount must be greater than 0");
      return;
    }

    if (Number.parseFloat(formData.amount) > balance) {
      setError("Insufficient balance");
      return;
    }

    const normalizedRecipient = normalizedRecipientAddress(formData.recipient);
    if (!normalizedRecipient) {
      setError("Please enter or scan a valid Ethereum address");
      return;
    }

    if (normalizedRecipient !== formData.recipient) {
      setFormData((prev) => ({ ...prev, recipient: normalizedRecipient }));
    }

    setStep("confirm");
  };

  const handleConfirm = async () => {
    const normalizedRecipient = normalizedRecipientAddress(formData.recipient);
    if (!normalizedRecipient) {
      setError("Please enter or scan a valid Ethereum address");
      return;
    }

    const amountNumber = Number.parseFloat(formData.amount);
    if (!Number.isFinite(amountNumber) || amountNumber <= 0) {
      setError("Amount must be greater than 0");
      return;
    }

    if (amountNumber > balance) {
      setError("Insufficient balance");
      return;
    }

    setW9Reason(null);
    setW9Year(null);
    setW9Email(null);
    setW9EmailInput("");
    setError("");

    setStep("sending");

    if (user?.isAdmin) {
      try {
        const amountWei = toAmountWei(formData.amount);
        const res = await authFetch("/w9/check", {
          method: "POST",
          body: JSON.stringify({
            from_address: wallet.address,
            to_address: normalizedRecipient,
            amount: amountWei.toString(),
          }),
        });

        if (res.status === 403) {
          const data = await res.json().catch(() => null);
          const reason: "w9_required" | "w9_pending" =
            data?.reason === "w9_pending" ? "w9_pending" : "w9_required";
          const email =
            typeof data?.email === "string" && data.email.trim()
              ? data.email.trim()
              : null;

          setW9Reason(reason);
          setW9Year(typeof data?.year === "number" ? data.year : null);
          setW9Email(email);
          setW9EmailInput(email || "");
          setError(
            reason === "w9_pending"
              ? "W9 submission is pending approval. Transfers are blocked until approved."
              : "W9 required before sending to this wallet.",
          );
          setStep("error");
          return;
        }

        if (res.status !== 200) {
          setError("Unable to validate W9 compliance. Please try again.");
          setStep("error");
          return;
        }
      } catch {
        setError("Unable to validate W9 compliance. Please try again.");
        setStep("error");
        return;
      }
    }

    const tipPromptState = resolveTipPrompt();
    await executeSend({
      recipient: normalizedRecipient,
      amount: formData.amount,
      memo: formData.memo,
      successToast: `Successfully sent ${formData.amount} ${SYMBOL} to ${normalizedRecipient.slice(0, 6)}...${normalizedRecipient.slice(-4)}`,
      onSuccess: () => {
        setTipPrompt(tipPromptState);
        setStep("success");
      },
    });
  };

  const handleClose = () => {
    stopScanner();
    scanLockedRef.current = false;
    setStep("form");
    setFlowMode(defaultFlow);
    setFormData({ recipient: "", amount: "", memo: "" });
    setError("");
    setScanError("");
    setScanInstruction("Point your camera at a QR code.");
    setProcessingDetectedQr(false);
    setRecipientMerchantName("");
    setRecipientIsMerchant(false);
    setRecipientMatchedPaymentWallet(false);
    setRecipientTipToAddress("");
    setLinkProvidedTipTo("");
    setLinkProvidedMerchantName("");
    setTipPrompt(null);
    setShowScanMoreOptions(false);
    setHash(null);
    setW9Email(null);
    setW9Reason(null);
    setW9Year(null);
    setW9EmailInput("");
    setW9Submitting(false);
    onOpenChange(false);
  };

  const handleBackFromConfirm = () => {
    setError("");
    if (flowMode === "scan") {
      setFormData((prev) => ({ ...prev, recipient: "", amount: "" }));
      setScanError("");
      setScanInstruction("Point your camera at a QR code.");
      setProcessingDetectedQr(false);
      setRecipientMerchantName("");
      setRecipientIsMerchant(false);
      setRecipientMatchedPaymentWallet(false);
      setLinkProvidedTipTo("");
      setLinkProvidedMerchantName("");
      scanLockedRef.current = false;
    }
    setStep("form");
  };

  const handleOpenTipPrompt = () => {
    if (!tipPrompt) {
      handleClose();
      return;
    }
    setError("");
    setStep("tip_prompt");
  };

  const handleSkipTip = () => {
    handleClose();
  };

  const handleSendTip = async () => {
    if (!tipPrompt) {
      handleClose();
      return;
    }

    const tipAmount = tipPrompt.amount.trim();
    if (!tipAmount) {
      setError("Enter a tip amount to continue.");
      return;
    }

    const tipAmountNumber = Number.parseFloat(tipAmount);
    const baseAmountNumber = Number.parseFloat(formData.amount);
    if (!Number.isFinite(tipAmountNumber) || tipAmountNumber <= 0) {
      setError("Tip amount must be greater than 0.");
      return;
    }
    if (tipAmountNumber > Math.max(balance - baseAmountNumber, 0)) {
      setError("Insufficient balance for this tip.");
      return;
    }

    setError("");
    setStep("tip_sending");
    await executeSend({
      recipient: tipPrompt.tipToAddress,
      amount: tipAmount,
      successToast: `Successfully sent a ${tipAmount} ${SYMBOL} tip to ${tipPrompt.merchantName}.`,
      onSuccess: () => {
        setTipPrompt((current) =>
          current ? { ...current, amount: tipAmount } : current,
        );
        setStep("tip_success");
      },
    });
  };

  const shortenAddress = (address: string, start = 6, end = 4) => {
    if (!address) return "";
    if (address.length <= start + end) return address;
    return `${address.slice(0, start)}...${address.slice(-end)}`;
  };

  const renderContent = () => {
    switch (step) {
      case "form":
        return (
          <div className="space-y-4">
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="flex items-center justify-end">
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={() =>
                    switchFlow(flowMode === "manual" ? "scan" : "manual")
                  }
                >
                  Switch to{" "}
                  {flowMode === "manual" ? "Scan Flow" : "Manual Flow"}
                </Button>
              </div>

              {flowMode === "manual" ? (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="recipient" className="text-sm font-medium">
                      Recipient Address *
                    </Label>
                    <ContactOrAddressInput
                      id="recipient"
                      initialValue={defaultRecipient}
                      onChange={(value) => {
                        // If the user pastes a payment link, lift out `to`
                        // and `tipTo` and store the address into the form
                        // (the contact-or-address-input would otherwise treat
                        // the whole URL as an unrecognized recipient).
                        const merchantSend =
                          extractMerchantSendFromPayload(value);
                        if (merchantSend) {
                          setFormData({
                            ...formData,
                            recipient: merchantSend.to,
                          });
                          setLinkProvidedTipTo(merchantSend.tipTo || "");
                          setLinkProvidedMerchantName("");
                          return;
                        }
                        setFormData({ ...formData, recipient: value });
                        // Clear any prior link-provided tipTo if the user is
                        // now editing the recipient by hand to a different
                        // value.
                        setLinkProvidedTipTo("");
                        setLinkProvidedMerchantName("");
                      }}
                      className="font-mono text-sm h-11"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="amount" className="text-sm font-medium">
                      Amount *
                    </Label>
                    <div className="relative">
                      <Input
                        id="amount"
                        type="number"
                        step="0.00000001"
                        placeholder="0.00"
                        value={formData.amount}
                        onChange={(e) =>
                          setFormData({ ...formData, amount: e.target.value })
                        }
                        className="h-11 pr-16"
                      />
                      <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground font-medium">
                        {SYMBOL}
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Available: {balance} {SYMBOL}
                    </p>
                  </div>
                </>
              ) : (
                <div className="space-y-3 rounded-lg border p-3">
                  <div className="space-y-2">
                    <Label className="text-sm font-medium">
                      Scan Recipient QR
                    </Label>
                    <div className="relative rounded-md bg-black/90 overflow-hidden border">
                      <video
                        ref={videoRef}
                        className="h-56 w-full object-cover"
                        muted
                        playsInline
                      />
                      {processingDetectedQr && (
                        <div className="pointer-events-none absolute inset-0">
                          <div className="absolute inset-4 rounded-lg border border-white/40 shadow-[0_0_24px_rgba(255,255,255,0.2)]" />
                          <div className="absolute left-4 right-4 h-0.5 bg-[#eb6c6c] shadow-[0_0_16px_rgba(235,108,108,0.85)] animate-qr-scan-line" />
                        </div>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {scanInstruction}
                    </p>
                  </div>

                  {!scannerRunning && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={startScanner}
                    >
                      <Camera className="h-4 w-4 mr-2" />
                      Start Camera
                    </Button>
                  )}

                  {!scannerSupported && (
                    <p className="text-xs text-amber-600">
                      Browser QR detection support is limited. You can switch to
                      manual flow at any time.
                    </p>
                  )}

                  <div className="space-y-2">
                    <Button
                      type="button"
                      variant="ghost"
                      className="h-9 px-0 !bg-transparent hover:!bg-transparent active:!bg-transparent focus:!bg-transparent focus-visible:!bg-transparent focus-visible:!ring-0 focus-visible:!ring-offset-0"
                      onClick={() => setShowScanMoreOptions((prev) => !prev)}
                    >
                      {showScanMoreOptions ? (
                        <ChevronUp className="h-4 w-4 mr-2" />
                      ) : (
                        <ChevronDown className="h-4 w-4 mr-2" />
                      )}
                      More options
                    </Button>
                    {showScanMoreOptions && (
                      <div className="space-y-3 rounded-md border bg-secondary/20 p-3">
                        <Button
                          type="button"
                          variant="outline"
                          onClick={openPhotoPicker}
                          disabled={photoScanLoading}
                          className="w-full"
                        >
                          <ImageUp className="h-4 w-4 mr-2" />
                          {photoScanLoading ? "Reading..." : "Camera Roll"}
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              )}

              {flowMode === "manual" && (
                <div className="space-y-2">
                  <Label htmlFor="memo" className="text-sm font-medium">
                    Memo (Optional)
                  </Label>
                  <Textarea
                    id="memo"
                    placeholder="Add a note for this transaction"
                    value={formData.memo}
                    onChange={(e) =>
                      setFormData({ ...formData, memo: e.target.value })
                    }
                    rows={3}
                    className="resize-none"
                  />
                </div>
              )}

              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handlePhotoInputChange}
              />

              {error && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              {scanError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{scanError}</span>
                </div>
              )}

              {flowMode === "manual" ? (
                <div className="flex flex-col gap-3 pt-4 border-t">
                  <Button type="submit" className="h-11 w-full">
                    Review Transaction
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleClose}
                    className="h-11 w-full bg-transparent"
                  >
                    Cancel
                  </Button>
                </div>
              ) : null}
            </form>
          </div>
        );

      case "confirm":
        return (
          <div className="space-y-3">
            {flowMode !== "scan" && (
              <div className="text-center pb-2">
                <h3 className="text-lg font-semibold mb-2">
                  Confirm Transaction
                </h3>
                <p className="text-muted-foreground text-sm">
                  Please review the details before sending
                </p>
              </div>
            )}

            {flowMode === "scan" ? (
              <Card className="overflow-hidden border-primary/25 bg-gradient-to-b from-primary/5 via-background to-background">
                <CardContent className="space-y-3 p-3 sm:p-4">
                  <div className="rounded-2xl border bg-background/90 p-3 shadow-sm">
                    <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground text-center">
                      Amount
                    </p>
                    <div className="mt-1.5 flex items-center justify-center gap-1">
                      <span className="text-2xl sm:text-3xl font-semibold text-foreground">
                        $
                      </span>
                      <Input
                        id="confirm-amount"
                        type="number"
                        step="0.01"
                        min="0"
                        inputMode="decimal"
                        placeholder="0.00"
                        value={formData.amount}
                        onChange={(e) =>
                          setFormData({ ...formData, amount: e.target.value })
                        }
                        onWheel={(e) => e.currentTarget.blur()}
                        className="h-auto w-full max-w-[200px] border-0 bg-transparent px-0 text-center text-2xl sm:text-3xl font-semibold focus-visible:ring-0"
                      />
                    </div>
                    <p className="mt-1 text-center text-xs text-muted-foreground">
                      {SYMBOL}
                    </p>
                    <p className="mt-2 text-center text-xs text-muted-foreground">
                      Available: {balance} {SYMBOL}
                    </p>
                  </div>

                  <div className="space-y-2 rounded-xl border bg-background/80 p-3">
                    <div>
                      <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
                        To
                      </p>
                      <p className="text-sm font-semibold leading-tight">
                        {contacts.find(
                          (contact) => contact.address === formData.recipient,
                        )?.name ||
                          recipientMerchantName ||
                          "Scanned Recipient"}
                      </p>
                      <p className="font-mono text-xs text-muted-foreground break-all">
                        {shortenAddress(formData.recipient, 8, 6)}
                      </p>
                    </div>
                    <div className="border-t pt-2">
                      <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
                        From
                      </p>
                      <p className="text-sm font-semibold leading-tight">
                        {wallet.name}
                      </p>
                    </div>
                  </div>

                  <div className="space-y-2 rounded-xl border bg-background/80 p-3">
                    <Label
                      htmlFor="confirm-memo"
                      className="text-[11px] uppercase tracking-wide text-muted-foreground"
                    >
                      Memo (Optional)
                    </Label>
                    <Input
                      id="confirm-memo"
                      type="text"
                      placeholder="Add a note"
                      value={formData.memo}
                      onChange={(e) =>
                        setFormData({ ...formData, memo: e.target.value })
                      }
                      className="h-10"
                    />
                  </div>
                </CardContent>
              </Card>
            ) : (
              <Card className="overflow-hidden border-primary/20 bg-gradient-to-b from-primary/5 via-background to-background">
                <CardContent className="space-y-3 p-3 sm:p-4">
                  <div className="rounded-xl border bg-background/90 p-3 shadow-sm">
                    <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                      Amount
                    </p>
                    <p className="mt-1 text-lg font-semibold sm:text-xl">
                      {formData.amount} {SYMBOL}
                    </p>
                  </div>

                  <div className="space-y-2 rounded-xl border bg-background/80 p-3">
                    <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
                      To
                    </p>
                    <p className="text-sm font-semibold leading-tight">
                      {contacts.find(
                        (contact) => contact.address === formData.recipient,
                      )?.name ||
                        recipientMerchantName ||
                        shortenAddress(formData.recipient, 8, 6)}
                    </p>
                    <p className="font-mono text-xs text-muted-foreground break-all">
                      {formData.recipient}
                    </p>
                  </div>

                  <div className="flex items-center justify-between gap-3 rounded-xl border bg-background/80 p-3">
                    <div>
                      <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
                        From
                      </p>
                      <p className="text-sm font-semibold leading-tight">
                        {wallet.name}
                      </p>
                    </div>
                    <Avatar className="h-8 w-8">
                      <AvatarImage
                        src={`/placeholder.svg?height=32&width=32&text=${wallet.name}`}
                      />
                      <AvatarFallback className="text-xs">
                        {wallet.name.slice(0, 2).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                  </div>

                  {formData.memo && (
                    <div className="space-y-1 rounded-xl border bg-background/80 p-3">
                      <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
                        Memo
                      </p>
                      <p className="text-sm break-words">{formData.memo}</p>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}

            {error && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div
              className={
                flowMode === "scan"
                  ? "grid grid-cols-2 gap-2 pt-3 border-t"
                  : "flex flex-col gap-3 pt-4 border-t"
              }
            >
              <Button onClick={handleConfirm} className="h-11 w-full">
                <Send className="h-4 w-4 mr-2" />
                Send
              </Button>
              <Button
                variant="outline"
                onClick={handleBackFromConfirm}
                className="h-11 w-full bg-transparent"
              >
                <ArrowLeft className="h-4 w-4 mr-2" />
                Back
              </Button>
            </div>
          </div>
        );

      case "sending":
        return (
          <div className="text-center space-y-6 py-8">
            <div className="h-16 w-16 mx-auto rounded-full bg-primary/10 flex items-center justify-center">
              <Send className="h-8 w-8 text-primary animate-pulse" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">
                Sending Transaction
              </h3>
              <p className="text-muted-foreground text-sm">
                Please wait while we process your transaction...
              </p>
            </div>
          </div>
        );

      case "success":
        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
              <CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Transaction Sent!</h3>
              <p className="text-muted-foreground text-sm mb-4">
                Your transaction has been broadcast to the network
              </p>
              <div className="space-y-2">
                <Label className="text-sm font-medium">Tranaction ID</Label>
                <div className="flex gap-2">
                  <Input
                    value={`${hash?.slice(0, 6)}...${hash?.slice(-4)}`}
                    readOnly
                    className="font-mono text-xs sm:text-sm h-11"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={copyHash}
                    className="px-3 bg-transparent h-11 flex-shrink-0"
                  >
                    {copied ? (
                      <CheckCircle className="h-4 w-4 text-green-600" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              </div>
            </div>
            {tipPrompt ? (
              <div className="flex flex-col gap-3">
                <Button onClick={handleOpenTipPrompt} className="w-full h-11">
                  Continue
                </Button>
                <Button
                  variant="outline"
                  onClick={handleSkipTip}
                  className="w-full h-11 bg-transparent"
                >
                  Done
                </Button>
              </div>
            ) : (
              <Button onClick={handleClose} className="w-full h-11">
                Done
              </Button>
            )}
          </div>
        );

      case "tip_prompt":
        return (
          <div className="space-y-5 py-2">
            <div className="text-center space-y-2">
              <h3 className="text-lg font-semibold">
                Thank you! Would you like to leave a tip?
              </h3>
              <p className="text-sm text-muted-foreground">
                {tipPrompt?.merchantName
                  ? `Add an optional second payment for ${tipPrompt.merchantName}.`
                  : "Add an optional second payment for this merchant."}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="tip-amount" className="text-sm font-medium">
                Tip Amount
              </Label>
              <div className="relative">
                <Input
                  id="tip-amount"
                  type="number"
                  step="0.00000001"
                  min="0"
                  placeholder="0.00"
                  value={tipPrompt?.amount || ""}
                  onChange={(e) =>
                    setTipPrompt((current) =>
                      current
                        ? { ...current, amount: e.target.value }
                        : current,
                    )
                  }
                  className="h-11 pr-16"
                />
                <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground font-medium">
                  {SYMBOL}
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                This tip is sent as a separate transfer to the merchant&apos;s
                tipping wallet.
              </p>
            </div>

            {error && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="flex flex-col gap-3">
              <Button onClick={handleSendTip} className="w-full h-11">
                Send Tip
              </Button>
              <Button
                variant="outline"
                onClick={handleSkipTip}
                className="w-full h-11 bg-transparent"
              >
                No thanks
              </Button>
            </div>
          </div>
        );

      case "tip_sending":
        return (
          <div className="text-center space-y-6 py-8">
            <div className="h-16 w-16 mx-auto rounded-full bg-primary/10 flex items-center justify-center">
              <Send className="h-8 w-8 text-primary animate-pulse" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Sending Tip</h3>
              <p className="text-muted-foreground text-sm">
                Submitting the separate tip payment…
              </p>
            </div>
          </div>
        );

      case "tip_success":
        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
              <CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Tip Sent!</h3>
              <p className="text-muted-foreground text-sm mb-4">
                {tipPrompt?.merchantName
                  ? `Your tip to ${tipPrompt.merchantName} has been broadcast to the network.`
                  : "Your tip has been broadcast to the network."}
              </p>
              <div className="space-y-2">
                <Label className="text-sm font-medium">Transaction ID</Label>
                <div className="flex gap-2">
                  <Input
                    value={`${hash?.slice(0, 6)}...${hash?.slice(-4)}`}
                    readOnly
                    className="font-mono text-xs sm:text-sm h-11"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={copyHash}
                    className="px-3 bg-transparent h-11 flex-shrink-0"
                  >
                    {copied ? (
                      <CheckCircle className="h-4 w-4 text-green-600" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              </div>
            </div>
            <Button onClick={handleClose} className="w-full h-11">
              Done
            </Button>
          </div>
        );

      case "error":
        if (w9Reason) {
          return (
            <div className="space-y-5 py-2">
              <div className="h-16 w-16 mx-auto rounded-full bg-amber-100 dark:bg-amber-900/20 flex items-center justify-center">
                <AlertTriangle className="h-8 w-8 text-amber-600 dark:text-amber-400" />
              </div>
              <div className="text-center space-y-2">
                <h3 className="text-lg font-semibold">W9 Approval Required</h3>
                <p className="text-sm text-muted-foreground break-all">
                  <span className="font-mono">{formData.recipient}</span> needs
                  to have an approved W9 form in order to receive more {SYMBOL}.
                </p>
                <p className="text-sm text-muted-foreground">
                  To pre-approve this user&apos;s W9 form, click approve below.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="w9-email" className="text-sm font-medium">
                  Recipient Email
                </Label>
                <Input
                  id="w9-email"
                  type="email"
                  value={w9EmailInput}
                  onChange={(e) => setW9EmailInput(e.target.value)}
                  placeholder="user@example.com"
                  className="h-11"
                />
                <p className="text-xs text-muted-foreground">
                  {w9Email
                    ? "Prefilled from existing records. You can edit it before approving."
                    : "No email found in W9 records. Enter recipient email to continue."}
                </p>
              </div>

              {error && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <div className="flex flex-col gap-3 pt-2">
                <Button
                  onClick={handleApproveAndSend}
                  className="w-full h-11"
                  disabled={w9Submitting}
                >
                  {w9Submitting ? "Approving..." : "Approve & Send"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setStep("form");
                    setError("");
                    setW9Reason(null);
                    setW9Year(null);
                    setW9EmailInput("");
                  }}
                  className="w-full h-11 bg-transparent"
                  disabled={w9Submitting}
                >
                  Cancel
                </Button>
              </div>
            </div>
          );
        }

        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
              <X className="h-8 w-8 text-red-600 dark:text-red-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Transaction Failed</h3>
              <p className="text-muted-foreground text-sm">{error}</p>
              {w9Email ? (
                <p className="text-sm mt-2">
                  Recipient email on file:{" "}
                  <span className="font-medium">{w9Email}</span>
                </p>
              ) : (
                <p className="text-sm mt-2 text-muted-foreground">
                  No recipient email on file.
                </p>
              )}
            </div>
            <div className="flex flex-col gap-3">
              <Button
                onClick={() => {
                  setStep("form");
                  setW9Reason(null);
                  setW9Year(null);
                  setW9EmailInput("");
                }}
                className="w-full h-11"
              >
                Try Again
              </Button>
              <Button
                variant="outline"
                onClick={handleClose}
                className="w-full h-11 bg-transparent"
              >
                Close
              </Button>
            </div>
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="mx-auto w-[calc(100vw-1rem)] max-w-md max-h-[calc(100dvh-1rem)] rounded-lg overflow-x-hidden overflow-y-auto p-4 sm:p-6">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">
            Send Cryptocurrency
          </DialogTitle>
          <DialogDescription className="text-sm">
            Send {SYMBOL} from your {wallet.name.toUpperCase()} wallet
          </DialogDescription>
        </DialogHeader>
        {renderContent()}
      </DialogContent>
    </Dialog>
  );
}
