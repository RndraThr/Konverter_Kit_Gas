import { useMutation } from '@tanstack/react-query';
import { Printer } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { previewVoucher, type SheetForm } from './api';
import { useFilePreview } from './FilePreviewDialog';
import { errorMessage } from './shared';

/** PDF sungguhan dari server, lengkap dengan lampiran, tanpa menyimpan voucher. */
export function VoucherPdfPreview({ form, blocked }: { form: SheetForm; blocked: boolean }) {
  const { show, viewer } = useFilePreview('Preview voucher');
  const load = useMutation({
    mutationFn: () => previewVoucher(form),
    onSuccess: show,
    onError: (error) => toast.error(errorMessage(error)),
  });
  return <>
    <Button type="button" variant="outline" disabled={blocked || load.isPending} onClick={() => load.mutate()}><Printer />{load.isPending ? 'Menyusun...' : 'Preview PDF'}</Button>
    {viewer}
  </>;
}
