import { ReactNode } from 'react';
import { useState } from 'react';
import { X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog';

type DialogLayout = 'compact' | 'wide' | 'workspace';

export function SetupFormSection({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <section className="setupFormSection"><header><h3>{title}</h3>{description && <p>{description}</p>}</header><div className="setupFormSectionFields">{children}</div></section>;
}

export function SetupDialog({ open, onOpenChange, title, description, children, pending, dirty = false, submitLabel = 'Simpan', onSubmit, layout = 'compact' }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string; children: ReactNode; pending?: boolean; dirty?: boolean; submitLabel?: string; onSubmit: () => void; layout?: DialogLayout }) {
  const [discardOpen, setDiscardOpen] = useState(false);
  const requestOpenChange = (nextOpen: boolean) => {
    if (nextOpen) return onOpenChange(true);
    if (pending) return;
    if (dirty) return setDiscardOpen(true);
    onOpenChange(false);
  };

  return <><Dialog open={open} onOpenChange={requestOpenChange}><DialogContent showCloseButton={false} aria-label={title} data-layout={layout} className="setupDialog max-h-[calc(100dvh-2rem)] max-w-none overflow-hidden p-0"><form className="flex h-full max-h-[calc(100dvh-2rem)] min-h-0 flex-col" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
    <DialogHeader className="relative shrink-0 border-b bg-background p-5 pr-16"><DialogTitle>{title}</DialogTitle><DialogDescription>{description}</DialogDescription><DialogClose render={<Button variant="ghost" size="icon" type="button" aria-label="Tutup" className="absolute top-3 right-3" />}><X /></DialogClose></DialogHeader>
    <div className="setupDialogBody">{children}</div>
    <DialogFooter className="setupDialogFooter sticky bottom-0 z-10 mx-0 mb-0 shrink-0 border-t bg-background"><DialogClose render={<Button variant="outline" type="button" disabled={pending} />}>Batal</DialogClose><Button disabled={pending} type="submit">{pending ? 'Menyimpan...' : submitLabel}</Button></DialogFooter>
  </form></DialogContent></Dialog>
    <AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Buang perubahan?</AlertDialogTitle><AlertDialogDescription>Perubahan pada formulir ini belum disimpan. Lanjutkan hanya jika Anda ingin menutup editor tanpa menyimpannya.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Lanjut mengedit</AlertDialogCancel><AlertDialogAction render={<Button type="button" variant="destructive" />} onClick={() => { setDiscardOpen(false); onOpenChange(false); }}>Buang perubahan</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </>;
}
