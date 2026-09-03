import { Dialog } from '@base-ui/react/dialog';
import { ReactNode } from 'react';
import { X } from 'lucide-react';

export function SetupDialog({ open, onOpenChange, title, description, children, pending, submitLabel = 'Simpan', onSubmit }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string; children: ReactNode; pending?: boolean; submitLabel?: string; onSubmit: () => void }) {
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal>
    <Dialog.Backdrop className="dialogBackdrop" />
    <Dialog.Popup className="dialogPopup" aria-label={title}>
      <form onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
        <header className="dialogHeader"><div><Dialog.Title>{title}</Dialog.Title><Dialog.Description>{description}</Dialog.Description></div><Dialog.Close className="iconButton" aria-label="Tutup"><X /></Dialog.Close></header>
        <div className="dialogBody">{children}</div>
        <footer className="dialogActions"><Dialog.Close className="secondaryButton">Batal</Dialog.Close><button className="primaryButton" disabled={pending}>{pending ? 'Menyimpan...' : submitLabel}</button></footer>
      </form>
    </Dialog.Popup>
  </Dialog.Portal></Dialog.Root>;
}
