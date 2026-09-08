import * as React from 'react';
import { AlertDialog as AlertDialogPrimitive } from '@base-ui/react/alert-dialog';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

function AlertDialog({ ...props }: AlertDialogPrimitive.Root.Props) {
  return <AlertDialogPrimitive.Root data-slot="alert-dialog" {...props} />;
}

function AlertDialogContent({ className, ...props }: AlertDialogPrimitive.Popup.Props) {
  return <AlertDialogPrimitive.Portal><AlertDialogPrimitive.Backdrop className="fixed inset-0 isolate z-50 bg-black/10 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0" /><AlertDialogPrimitive.Popup data-slot="alert-dialog-content" className={cn('fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl bg-popover p-5 text-sm text-popover-foreground ring-1 ring-foreground/10 outline-none sm:max-w-md data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95', className)} {...props} /></AlertDialogPrimitive.Portal>;
}

function AlertDialogHeader({ className, ...props }: React.ComponentProps<'div'>) { return <div className={cn('grid gap-2', className)} {...props} />; }
function AlertDialogFooter({ className, ...props }: React.ComponentProps<'div'>) { return <div className={cn('flex flex-col-reverse gap-2 sm:flex-row sm:justify-end', className)} {...props} />; }
function AlertDialogTitle({ className, ...props }: AlertDialogPrimitive.Title.Props) { return <AlertDialogPrimitive.Title className={cn('text-base font-semibold', className)} {...props} />; }
function AlertDialogDescription({ className, ...props }: AlertDialogPrimitive.Description.Props) { return <AlertDialogPrimitive.Description className={cn('text-sm leading-6 text-muted-foreground', className)} {...props} />; }
function AlertDialogCancel({ ...props }: AlertDialogPrimitive.Close.Props) { return <AlertDialogPrimitive.Close render={<Button type="button" variant="outline" />} {...props} />; }
function AlertDialogAction({ ...props }: AlertDialogPrimitive.Close.Props) { return <AlertDialogPrimitive.Close render={<Button type="button" />} {...props} />; }

export { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle };
