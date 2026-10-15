// Placeholder: replaced entirely by Task 13.
export default function FinalReportPresent({ onClose }: { programId: string; regencyId: string; onClose: () => void }) {
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-background">
    <button type="button" onClick={onClose}>Tutup</button>
  </div>;
}
