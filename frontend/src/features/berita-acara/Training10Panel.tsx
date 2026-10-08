import type { ProgramType } from '../programs/types';
import { TrainingPanel, type TrainingConfig } from './TrainingPanel';

const training10Config: TrainingConfig = {
  apiPath: 'training-10',
  name: 'Training 10%',
  documentTitle: 'BA Training 10%',
  locationKey: 'training_10_location',
  folder: '8. TRAINING 10%',
  filenamePrefix: 'BA-TRAINING-10%',
  participantRule: 'Satu dokumen per tanggal: 10% pertama (dibulatkan ke atas) dari daftar Training 100% hari itu, urut nomor bagi DP3. Tanda tangan peserta sementara dummy.',
};

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };

export function Training10Panel({ scheduleID, regencyName, programType }: Props) {
  return <TrainingPanel config={training10Config} scheduleID={scheduleID} regencyName={regencyName} programType={programType} />;
}
