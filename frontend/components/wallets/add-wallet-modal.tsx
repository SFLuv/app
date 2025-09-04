"use client"

import type React from "react"

import { useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Plus, Wallet } from "lucide-react"
import { type WalletType, walletTypeLabels } from "@/types/wallet"
import { useToast } from "@/hooks/use-toast"

interface AddWalletModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AddWalletModal({ open, onOpenChange }: AddWalletModalProps) {
  const [method, setMethod] = useState<"create" | "import">("create")
  const [formData, setFormData] = useState({
    name: "",
    type: "metamask" as WalletType,
    address: "",
    privateKey: "",
    seedPhrase: "",
    isDefault: false,
  })
  const [isLoading, setIsLoading] = useState(false)
  const { toast } = useToast()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsLoading(true)

    // Mock wallet creation/import delay
    await new Promise((resolve) => setTimeout(resolve, 2000))

    setIsLoading(false)
    toast({
      title: "Wallet Added Successfully",
      description: `${formData.name} has been added to your wallet list.`,
    })

    // Reset form
    setFormData({
      name: "",
      type: "metamask",
      address: "",
      privateKey: "",
      seedPhrase: "",
      isDefault: false,
    })
    onOpenChange(false)
  }

  const generateWallet = () => {
    // Mock wallet generation
    const mockAddress = "0x" + Math.random().toString(16).substr(2, 40)
    setFormData((prev) => ({
      ...prev,
      address: mockAddress,
      name: prev.name || `${walletTypeLabels[prev.type]} Wallet`,
    }))
    toast({
      title: "Wallet Generated",
      description: "A new wallet address has been generated.",
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Plus className="h-5 w-5" />
            Add New Wallet
          </DialogTitle>
          <DialogDescription>Create a new wallet or import an existing one</DialogDescription>
        </DialogHeader>

        <Tabs value={method} onValueChange={(value) => setMethod(value as "create" | "import")}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="create">Create New</TabsTrigger>
            <TabsTrigger value="import">Import Existing</TabsTrigger>
          </TabsList>

          <form onSubmit={handleSubmit} className="space-y-4">
            <TabsContent value="create" className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Wallet Name</Label>
                <Input
                  id="name"
                  placeholder="My Wallet"
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="type">Wallet Type</Label>
                <Select
                  value={formData.type}
                  onValueChange={(value: WalletType) => setFormData((prev) => ({ ...prev, type: value }))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(walletTypeLabels).map(([key, label]) => (
                      <SelectItem key={key} value={key}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <Card>
                <CardContent className="pt-6">
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="font-medium">Generate New Address</p>
                        <p className="text-sm text-muted-foreground">Create a new wallet address automatically</p>
                      </div>
                      <Button type="button" variant="outline" onClick={generateWallet}>
                        <Wallet className="h-4 w-4 mr-2" />
                        Generate
                      </Button>
                    </div>

                    {formData.address && (
                      <div className="space-y-2">
                        <Label>Generated Address</Label>
                        <Input value={formData.address} readOnly className="font-mono text-sm" />
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="import" className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="import-name">Wallet Name</Label>
                <Input
                  id="import-name"
                  placeholder="Imported Wallet"
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="import-type">Wallet Type</Label>
                <Select
                  value={formData.type}
                  onValueChange={(value: WalletType) => setFormData((prev) => ({ ...prev, type: value }))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(walletTypeLabels).map(([key, label]) => (
                      <SelectItem key={key} value={key}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <Tabs defaultValue="address" className="w-full">
                <TabsList className="grid w-full grid-cols-3">
                  <TabsTrigger value="address">Address</TabsTrigger>
                  <TabsTrigger value="private-key">Private Key</TabsTrigger>
                  <TabsTrigger value="seed-phrase">Seed Phrase</TabsTrigger>
                </TabsList>

                <TabsContent value="address" className="space-y-2">
                  <Label htmlFor="address">Wallet Address</Label>
                  <Input
                    id="address"
                    placeholder="0x..."
                    value={formData.address}
                    onChange={(e) => setFormData((prev) => ({ ...prev, address: e.target.value }))}
                    required
                  />
                </TabsContent>

                <TabsContent value="private-key" className="space-y-2">
                  <Label htmlFor="private-key">Private Key</Label>
                  <Input
                    id="private-key"
                    type="password"
                    placeholder="Enter your private key"
                    value={formData.privateKey}
                    onChange={(e) => setFormData((prev) => ({ ...prev, privateKey: e.target.value }))}
                  />
                </TabsContent>

                <TabsContent value="seed-phrase" className="space-y-2">
                  <Label htmlFor="seed-phrase">Seed Phrase</Label>
                  <Input
                    id="seed-phrase"
                    placeholder="Enter your 12 or 24 word seed phrase"
                    value={formData.seedPhrase}
                    onChange={(e) => setFormData((prev) => ({ ...prev, seedPhrase: e.target.value }))}
                  />
                </TabsContent>
              </Tabs>
            </TabsContent>

            <div className="flex items-center space-x-2">
              <Switch
                id="default"
                checked={formData.isDefault}
                onCheckedChange={(checked) => setFormData((prev) => ({ ...prev, isDefault: checked }))}
              />
              <Label htmlFor="default">Set as default wallet</Label>
            </div>

            <div className="flex gap-2 pt-4">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} className="flex-1">
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={isLoading || !formData.name || (!formData.address && method === "import")}
                className="flex-1"
              >
                {isLoading ? "Adding..." : `${method === "create" ? "Create" : "Import"} Wallet`}
              </Button>
            </div>
          </form>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
