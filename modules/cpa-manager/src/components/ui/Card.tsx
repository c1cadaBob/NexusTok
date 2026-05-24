import type { PropsWithChildren, ReactNode } from 'react';

interface CardProps {
  title?: ReactNode;
  extra?: ReactNode;
  className?: string;
  id?: string;
}

export function Card({ title, extra, children, className, id }: PropsWithChildren<CardProps>) {
  return (
    <div id={id} className={className ? `card ${className}` : 'card'}>
      {(title || extra) && (
        <div className="card-header">
          <div className="title">{title}</div>
          {extra}
        </div>
      )}
      {children}
    </div>
  );
}
