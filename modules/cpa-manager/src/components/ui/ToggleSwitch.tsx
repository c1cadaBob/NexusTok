import { useId, type ChangeEvent, type ReactNode } from 'react';
import styles from './ToggleSwitch.module.scss';

interface ToggleSwitchProps {
  checked: boolean;
  onChange: (value: boolean) => void;
  label?: ReactNode;
  ariaLabel?: string;
  disabled?: boolean;
  labelPosition?: 'left' | 'right';
  id?: string;
  name?: string;
}

export function ToggleSwitch({
  checked,
  onChange,
  label,
  ariaLabel,
  disabled = false,
  labelPosition = 'right',
  id,
  name
}: ToggleSwitchProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const inputName = name ?? inputId;

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange(event.target.checked);
  };

  const className = [
    styles.root,
    labelPosition === 'left' ? styles.labelLeft : '',
    disabled ? styles.disabled : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <label className={className}>
      <input
        id={inputId}
        name={inputName}
        type="checkbox"
        checked={checked}
        onChange={handleChange}
        disabled={disabled}
        aria-label={ariaLabel}
      />
      <span className={styles.track}>
        <span className={styles.thumb} />
      </span>
      {label && <span className={styles.label}>{label}</span>}
    </label>
  );
}
