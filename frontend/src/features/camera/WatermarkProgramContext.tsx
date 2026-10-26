import { createContext, useContext, type ReactNode } from 'react'

const WatermarkProgramContext = createContext<string | undefined>(undefined)

/** Supplies the program whose tender logo the watermark camera should use. */
export function WatermarkProgramProvider({ programId, children }: { programId?: string; children: ReactNode }) {
  return <WatermarkProgramContext.Provider value={programId || undefined}>{children}</WatermarkProgramContext.Provider>
}

export function useWatermarkProgram(): string | undefined {
  return useContext(WatermarkProgramContext)
}
