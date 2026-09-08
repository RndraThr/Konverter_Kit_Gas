import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { expect, test } from 'vitest';
import { ColumnMappingStep } from './ColumnMappingStep';
import { DCP3Mapping, emptyMapping } from './types';

const collisionHeader = '__dcp3_unmapped__';

function MappingHarness() {
  const [mapping, setMapping] = useState<DCP3Mapping>(emptyMapping);
  return <>
    <ColumnMappingStep headers={[collisionHeader]} mapping={mapping} programType="farmer" onChange={setMapping} />
    <pre aria-label="Mapping JSON">{JSON.stringify(mapping)}</pre>
  </>;
}

async function chooseSource(optionName: string) {
  await userEvent.click(screen.getByRole('combobox', { name: /Nomor urut DCP3/ }));
  await userEvent.click(await screen.findByRole('option', { name: optionName }));
}

test('keeps an unmap-like workbook header distinct from the explicit unmap option', async () => {
  render(<MappingHarness />);

  await chooseSource(collisionHeader);
  expect(screen.getByLabelText('Mapping JSON')).toHaveTextContent(`"source_sequence":"${collisionHeader}"`);

  await chooseSource('Tidak dipetakan');
  expect(screen.getByLabelText('Mapping JSON')).toHaveTextContent('"source_sequence":""');
});
