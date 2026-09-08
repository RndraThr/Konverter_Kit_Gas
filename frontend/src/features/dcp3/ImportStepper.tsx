import { Check } from 'lucide-react';
import styles from './DCP3Import.module.css';

type ImportStep = {
  label: string;
  description: string;
};

type ImportStepperProps = {
  currentStep: number;
  steps: ImportStep[];
};

const stateLabel = {
  complete: 'Selesai',
  current: 'Saat ini',
  upcoming: 'Berikutnya',
};

export function ImportStepper({ currentStep, steps }: ImportStepperProps) {
  return <ol className={styles.steps} aria-label="Tahapan import DCP3">
    {steps.map((step, index) => {
      const number = index + 1;
      const state = number < currentStep ? 'complete' : number === currentStep ? 'current' : 'upcoming';

      return <li
        key={step.label}
        data-state={state}
        aria-current={state === 'current' ? 'step' : undefined}
      >
        <span className={styles.stepNumber} aria-hidden="true">
          {state === 'complete' ? <Check /> : number}
        </span>
        <span className={styles.stepCopy}>
          <span className={styles.stepState}>{stateLabel[state]}</span>
          <strong>{step.label}</strong>
          <small>{step.description}</small>
        </span>
      </li>;
    })}
  </ol>;
}
