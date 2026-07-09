'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useDashboard } from '@/hooks/useDashboard'
import { useScanStatus, useStartScan } from '@/hooks/useScan'
import { 
  CheckCircle2, 
  Circle, 
  HardDrive, 
  Copy, 
  FolderTree, 
  Rocket,
  ChevronRight,
  Loader2,
  RefreshCw,
  Search,
  Sparkles
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'

const ONBOARDING_KEY = 'plex2jellyfin_onboarding_completed'

interface Step {
  id: number
  title: string
  description: string
  icon: React.ReactNode
  accentColor: string
}

const steps: Step[] = [
  {
    id: 1,
    title: 'Scan Libraries',
    description: 'Index your media libraries to discover movies, TV shows, and potential issues.',
    icon: <Search className="h-8 w-8" />,
    accentColor: 'from-blue-500/20 to-cyan-500/20 text-cyan-400'
  },
  {
    id: 2,
    title: 'Review Duplicates',
    description: 'Find and manage duplicate files to reclaim storage space.',
    icon: <Copy className="h-8 w-8" />,
    accentColor: 'from-amber-500/20 to-orange-500/20 text-orange-400'
  },
  {
    id: 3,
    title: 'Consolidate Series',
    description: 'Organize scattered TV series episodes into proper folder structures.',
    icon: <FolderTree className="h-8 w-8" />,
    accentColor: 'from-violet-500/20 to-purple-500/20 text-purple-400'
  },
  {
    id: 4,
    title: 'Ready to Go',
    description: 'Your library is organized and ready for Jellyfin!',
    icon: <Rocket className="h-8 w-8" />,
    accentColor: 'from-emerald-500/20 to-green-500/20 text-emerald-400'
  },
]

export default function OnboardingPage() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(1)
  const { data: dashboard, isLoading: dashboardLoading } = useDashboard()
  const { data: scanStatus } = useScanStatus()
  const startScan = useStartScan()

  // Check if onboarding already completed
  useEffect(() => {
    if (typeof window !== 'undefined') {
      const completed = localStorage.getItem(ONBOARDING_KEY)
      if (completed === 'true') {
        router.push('/')
      }
    }
  }, [router])

  const handleStartScan = () => {
    startScan.mutate()
  }

  const handleComplete = () => {
    localStorage.setItem(ONBOARDING_KEY, 'true')
    router.push('/')
  }

  const handleSkip = () => {
    localStorage.setItem(ONBOARDING_KEY, 'true')
    router.push('/')
  }

  const goToNext = () => {
    if (currentStep < steps.length) {
      setCurrentStep(currentStep + 1)
    }
  }

  const goToPrev = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1)
    }
  }

  const isScanning = scanStatus?.status === 'scanning'
  const scanComplete = (dashboard?.libraryStats?.totalFiles ?? 0) > 0

  return (
    <div className="min-h-screen bg-zinc-950 flex flex-col items-center justify-center p-4 relative overflow-hidden">
      
      <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-blue-500/5 rounded-full blur-3xl" />
      <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-violet-500/5 rounded-full blur-3xl" />

      <div className="w-full max-w-3xl relative z-10 flex flex-col items-center">
        {/* Header */}
        <div className="text-center mb-12 animate-in slide-in-from-bottom-4 fade-in duration-500">
          <div className="inline-flex items-center justify-center p-2 mb-4 rounded-xl bg-gradient-to-br from-zinc-800 to-zinc-900 border border-zinc-700/50 shadow-xl">
            <Sparkles className="w-6 h-6 text-violet-400" />
          </div>
          <h1 className="text-4xl font-bold text-transparent bg-clip-text bg-gradient-to-br from-white to-zinc-400 mb-3">
            Welcome to Plex2Jellyfin
          </h1>
          <p className="text-lg text-zinc-400 max-w-md mx-auto">
            Let's get your media library organized and optimized for Jellyfin
          </p>
        </div>

        
        <div className="w-full max-w-xl mb-12">
          <div className="relative flex justify-between">
            
            <div className="absolute top-1/2 left-0 right-0 h-0.5 -translate-y-1/2 bg-zinc-800 z-0" />
            <div 
              className="absolute top-1/2 left-0 h-0.5 -translate-y-1/2 bg-violet-500 z-0 transition-all duration-500 ease-in-out" 
              style={{ width: `${((currentStep - 1) / (steps.length - 1)) * 100}%` }}
            />

            {steps.map((step) => {
              const isActive = currentStep === step.id
              const isPast = currentStep > step.id
              
              return (
                <div key={step.id} className="relative z-10 flex flex-col items-center group">
                  <div
                    className={cn(
                      "flex items-center justify-center w-12 h-12 rounded-xl border-2 transition-all duration-500 ease-in-out shadow-sm",
                      isPast 
                        ? "bg-violet-500 border-violet-500 text-white shadow-violet-500/20" 
                        : isActive 
                          ? "bg-zinc-900 border-violet-500 text-violet-400 scale-110 shadow-violet-500/20" 
                          : "bg-zinc-900 border-zinc-800 text-zinc-500"
                    )}
                  >
                    {isPast ? (
                      <CheckCircle2 className="h-6 w-6" />
                    ) : (
                      <span className="text-sm font-semibold">{step.id}</span>
                    )}
                  </div>
                  <div className={cn(
                    "absolute -bottom-8 w-max text-xs font-medium transition-colors duration-300",
                    isActive ? "text-violet-400" : isPast ? "text-zinc-300" : "text-zinc-600"
                  )}>
                    {step.title}
                  </div>
                </div>
              )
            })}
          </div>
        </div>

        
        <div className="w-full relative min-h-[400px]">
          {steps.map((step) => (
            <div 
              key={step.id}
              className={cn(
                "absolute inset-0 transition-all duration-500 ease-in-out",
                currentStep === step.id 
                  ? "opacity-100 translate-x-0 scale-100 pointer-events-auto" 
                  : currentStep > step.id 
                    ? "opacity-0 -translate-x-8 scale-95 pointer-events-none" 
                    : "opacity-0 translate-x-8 scale-95 pointer-events-none"
              )}
            >
              <Card className="h-full bg-zinc-900/80 backdrop-blur border-zinc-800 shadow-2xl flex flex-col overflow-hidden">
                <div className={cn("h-2 w-full bg-gradient-to-r", step.accentColor.split(' ')[0], step.accentColor.split(' ')[1])} />
                
                <div className="flex flex-col md:flex-row h-full">
                  
                  <div className={cn(
                    "md:w-1/3 p-8 flex flex-col items-center justify-center bg-gradient-to-b border-b md:border-b-0 md:border-r border-zinc-800/50",
                    step.accentColor.split(' ')[0],
                    step.accentColor.split(' ')[1]
                  )}>
                    <div className={cn(
                      "p-6 rounded-2xl bg-zinc-950/50 shadow-inner backdrop-blur-sm border border-white/5",
                      step.accentColor.split(' ')[2]
                    )}>
                      {step.icon}
                    </div>
                  </div>

                  
                  <div className="flex-1 p-8 flex flex-col">
                    <div className="mb-6">
                      <h2 className="text-2xl font-bold text-white mb-2">{step.title}</h2>
                      <p className="text-zinc-400">{step.description}</p>
                    </div>

                    <div className="flex-1 flex flex-col justify-center">
                      
                      {step.id === 1 && (
                        <div className="space-y-4 w-full">
                          {dashboardLoading ? (
                            <div className="flex flex-col items-center justify-center py-8 space-y-4">
                              <Loader2 className="h-8 w-8 animate-spin text-zinc-500" />
                              <p className="text-sm text-zinc-500">Checking library status...</p>
                            </div>
                          ) : (
                            <>
                              {scanComplete && (
                                <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-xl p-5 animate-in fade-in zoom-in-95 duration-300">
                                  <div className="flex items-center gap-3 text-emerald-400 mb-4">
                                    <CheckCircle2 className="h-6 w-6" />
                                    <span className="font-semibold">Library scan complete!</span>
                                  </div>
                                  <div className="grid grid-cols-2 gap-y-4 gap-x-6 text-sm">
                                    <div className="flex flex-col">
                                      <span className="text-zinc-500">Total Files</span>
                                      <span className="text-white font-medium text-lg">
                                        {dashboard?.libraryStats?.totalFiles?.toLocaleString() || 0}
                                      </span>
                                    </div>
                                    <div className="flex flex-col">
                                      <span className="text-zinc-500">Library Size</span>
                                      <span className="text-white font-medium text-lg">
                                        {formatBytes(dashboard?.libraryStats?.totalSize || 0)}
                                      </span>
                                    </div>
                                    <div className="flex flex-col">
                                      <span className="text-zinc-500">Duplicates</span>
                                      <span className="text-white font-medium text-lg">
                                        {dashboard?.libraryStats?.duplicateGroups?.toLocaleString() || 0}
                                      </span>
                                    </div>
                                  </div>
                                </div>
                              )}

                              {isScanning && (
                                <div className="bg-blue-500/5 border border-blue-500/10 rounded-xl p-5 space-y-4">
                                  <div className="flex items-center justify-between text-blue-400">
                                    <div className="flex items-center gap-3">
                                      <RefreshCw className="h-5 w-5 animate-spin" />
                                      <span className="font-medium">Scanning libraries...</span>
                                    </div>
                                    <span className="text-sm font-medium">{scanStatus?.progress || 0}%</span>
                                  </div>
                                  <Progress value={scanStatus?.progress || 0} className="h-2 bg-zinc-800" />
                                  <p className="text-xs text-zinc-500 truncate">{scanStatus?.message || 'Processing files...'}</p>
                                </div>
                              )}

                              {!scanComplete && !isScanning && (
                                <div className="text-center py-6 bg-zinc-950/50 rounded-xl border border-zinc-800/50">
                                  <HardDrive className="h-10 w-10 text-zinc-600 mx-auto mb-4" />
                                  <p className="text-zinc-400 mb-6 max-w-xs mx-auto text-sm">
                                    Click the button below to start scanning your configured media libraries.
                                  </p>
                                  <Button
                                    onClick={handleStartScan}
                                    disabled={startScan.isPending}
                                    className="bg-cyan-600 hover:bg-cyan-700 text-white border-0 shadow-lg shadow-cyan-900/20"
                                  >
                                    {startScan.isPending ? (
                                      <>
                                        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                                        Starting Scan...
                                      </>
                                    ) : (
                                      <>
                                        <Search className="h-4 w-4 mr-2" />
                                        Start Library Scan
                                      </>
                                    )}
                                  </Button>
                                </div>
                              )}
                            </>
                          )}
                        </div>
                      )}

                      
                      {step.id === 2 && (
                        <div className="space-y-6">
                          <div className="bg-gradient-to-br from-zinc-800/50 to-zinc-900 rounded-xl p-6 border border-zinc-800">
                            <div className="flex items-center justify-between mb-2">
                              <span className="text-zinc-400 font-medium">Duplicate Groups Found</span>
                              <span className="text-4xl font-bold text-white tracking-tight">
                                {dashboard?.libraryStats?.duplicateGroups || 0}
                              </span>
                            </div>
                            {(dashboard?.libraryStats?.duplicateGroups || 0) > 0 ? (
                              <p className="text-sm text-amber-400 flex items-center gap-2 mt-4">
                                <Copy className="h-4 w-4" /> Found duplicates you can review
                              </p>
                            ) : (
                              <p className="text-sm text-emerald-400 flex items-center gap-2 mt-4">
                                <CheckCircle2 className="h-4 w-4" /> No duplicates found. Your library is clean!
                              </p>
                            )}
                          </div>
                          {(dashboard?.libraryStats?.duplicateGroups || 0) > 0 && (
                            <Button
                              variant="outline"
                              className="w-full border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white h-12"
                              onClick={() => router.push('/duplicates')}
                            >
                              Review Duplicates
                              <ChevronRight className="h-4 w-4 ml-2" />
                            </Button>
                          )}
                        </div>
                      )}

                      
                      {step.id === 3 && (
                        <div className="space-y-6">
                          <div className="bg-gradient-to-br from-zinc-800/50 to-zinc-900 rounded-xl p-6 border border-zinc-800">
                            <div className="flex items-center justify-between mb-2">
                              <span className="text-zinc-400 font-medium">Scattered Series</span>
                              <span className="text-4xl font-bold text-white tracking-tight">
                                {dashboard?.libraryStats?.scatteredSeries || 0}
                              </span>
                            </div>
                            <p className="text-sm text-zinc-500 mt-4 leading-relaxed">
                              Series with episodes spread across multiple directories or not properly organized in season folders.
                            </p>
                          </div>
                          {(dashboard?.libraryStats?.scatteredSeries || 0) > 0 ? (
                            <Button
                              variant="outline"
                              className="w-full border-zinc-700 text-zinc-300 hover:bg-zinc-800 hover:text-white h-12"
                              onClick={() => router.push('/consolidation')}
                            >
                              Review Scattered Series
                              <ChevronRight className="h-4 w-4 ml-2" />
                            </Button>
                          ) : (
                            <div className="flex items-center justify-center p-4 bg-emerald-500/10 rounded-lg border border-emerald-500/20 text-emerald-400 text-sm font-medium">
                              <CheckCircle2 className="h-4 w-4 mr-2" />
                              All series are properly organized!
                            </div>
                          )}
                        </div>
                      )}

                      
                      {step.id === 4 && (
                        <div className="text-center space-y-6 py-6 flex flex-col items-center">
                          <div className="relative">
                            
                            <div className="absolute inset-0 bg-emerald-500/20 rounded-full blur-xl animate-pulse" />
                            <div className="relative inline-flex items-center justify-center w-24 h-24 rounded-full bg-gradient-to-br from-emerald-400/20 to-emerald-600/20 border border-emerald-500/30 text-emerald-400 mb-2 shadow-2xl shadow-emerald-900/50">
                              <CheckCircle2 className="h-12 w-12" />
                            </div>
                          </div>
                          <div>
                            <h3 className="text-2xl font-bold text-white mb-3">You're all set!</h3>
                            <p className="text-zinc-400 max-w-sm mx-auto leading-relaxed">
                              Your media library is now being monitored. Plex2Jellyfin will automatically
                              organize new downloads and keep everything Jellyfin-ready.
                            </p>
                          </div>
                          <div className="pt-4 w-full">
                            <Button
                              onClick={handleComplete}
                              className="w-full sm:w-auto px-8 h-12 text-base bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-900/20 transition-all hover:scale-105"
                            >
                              Go to Dashboard
                              <Rocket className="h-4 w-4 ml-2" />
                            </Button>
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </Card>
            </div>
          ))}
        </div>

        
        <div className="w-full flex items-center justify-between mt-12 pt-6 border-t border-zinc-800/50">
          <Button
            variant="ghost"
            onClick={handleSkip}
            className="text-zinc-500 hover:text-zinc-300"
          >
            Skip Setup
          </Button>

          <div className="flex gap-3">
            <Button
              variant="outline"
              onClick={goToPrev}
              disabled={currentStep === 1}
              className="border-zinc-800 text-zinc-400 hover:bg-zinc-800 hover:text-white"
            >
              Back
            </Button>
            
            {currentStep < steps.length && (
              <Button
                onClick={goToNext}
                className="bg-violet-600 hover:bg-violet-500 text-white shadow-lg shadow-violet-900/20"
              >
                Continue
                <ChevronRight className="h-4 w-4 ml-2" />
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

