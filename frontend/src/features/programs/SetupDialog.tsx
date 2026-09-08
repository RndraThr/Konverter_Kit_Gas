import { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog';

export function SetupDialog({ open, onOpenChange, title, description, children, pending, submitLabel = 'Simpan', onSubmit }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string; children: ReactNode; pending?: boolean; submitLabel?: string; onSubmit: () => void }) {
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent showCloseButton={false} aria-label={title} className="max-h-[calc(100dvh-2rem)] max-w-3xl overflow-y-auto p-0"><form onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
    <DialogHeader className="relative border-b p-5 pr-16"><DialogTitle>{title}</DialogTitle><DialogDescription>{description}</DialogDescription><DialogClose render={<Button variant="ghost" size="icon" type="button" aria-label="Tutup" className="absolute top-3 right-3" />}><X /></DialogClose></DialogHeader>
    <div className="grid gap-4 p-5 sm:grid-cols-2">{children}</div>
    <DialogFooter className="mx-0 mb-0"><DialogClose render={<Button variant="outline" type="button" />}>Batal</DialogClose><Button disabled={pending} type="submit">{pending ? 'Menyimpan...' : submitLabel}</Button></DialogFooter>
  </form></DialogContent></Dialog>;
}
