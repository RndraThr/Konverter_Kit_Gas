import { useLayoutEffect, useRef } from 'react';
import { Input } from '@/components/ui/input';

const groupDigits = (value: number) => value > 0 ? new Intl.NumberFormat('id-ID').format(value) : '';
const digitsOf = (text: string) => text.replace(/\D/g, '');

// Di atas batas ini server menolak nilainya.
const maxDigits = 13;

// plain: tanpa gaya kolom form, untuk sel tabel.
type MoneyInputProps = { value: number; disabled?: boolean; plain?: boolean; className?: string; 'aria-label'?: string; onChange: (value: number) => void };

/** Rupiah dengan titik ribuan saat diketik; kursor tetap di digit yang sama. */
export function MoneyInput({ value, plain = false, onChange, ...rest }: MoneyInputProps) {
  const Control = plain ? 'input' : Input;
  const input = useRef<HTMLInputElement>(null);
  const caretDigits = useRef<number | null>(null);

  useLayoutEffect(() => {
    const element = input.current;
    const digits = caretDigits.current;
    if (!element || digits === null) return;
    caretDigits.current = null;
    let position = 0;
    for (let seen = 0; seen < digits && position < element.value.length; position += 1) {
      if (/\d/.test(element.value[position])) seen += 1;
    }
    element.setSelectionRange(position, position);
  });

  return <Control
    ref={input}
    inputMode="numeric"
    value={groupDigits(value)}
    {...rest}
    onChange={(event) => {
      const { value: text, selectionStart } = event.target;
      caretDigits.current = digitsOf(text.slice(0, selectionStart ?? text.length)).length;
      onChange(Number(digitsOf(text).slice(0, maxDigits)));
    }}
  />;
}
