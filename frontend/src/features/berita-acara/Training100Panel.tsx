import type { ProgramType } from '../programs/types';
import { TrainingPanel, type TrainingConfig } from './TrainingPanel';

const training100Config: TrainingConfig = {
  apiPath: 'training-100',
  name: 'Training 100%',
  documentTitle: 'BA Training 100%',
  locationKey: 'training_100_location',
  folder: '9. TRAINING 100%',
  filenamePrefix: 'BA-TRAINING-100%',
  participantRule: 'Satu dokumen per tanggal: semua penerima hari itu, urut nomor bagi DP3. Tanda tangan peserta sementara dummy.',
};

type Props = { scheduleID: string; regencyName: string; programType: ProgramType; defaultDate: string };

export function Training100Panel({ scheduleID, regencyName, programType }: Props) {
  return <TrainingPanel config={training100Config} scheduleID={scheduleID} regencyName={regencyName} programType={programType} />;
}
