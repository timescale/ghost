// SVG icons used by the schema tree. Sourced from the popsql lollipop
// beta-icon set so the appearance matches popsql exactly.

interface IconProps {
  className?: string;
  size?: number;
}

export function ChevronDown({ className = '', size = 16 }: IconProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M5 7L8 10L11 7"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

// NavTable from popsql-lollipop (14x14 viewbox). Used for table/view groups
// and for individual table/view rows.
export function NavTable({ className = '', size = 14 }: IconProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 14 14"
      fill="none"
      aria-hidden="true"
    >
      <path
        clipRule="evenodd"
        d="M12 0H2a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2V2a2 2 0 00-2-2zm.5 4.25V2a.5.5 0 00-.5-.5H2a.5.5 0 00-.5.5v2.25h11zm-11 1.5v2.5h4.75v-2.5H1.5zm0 4V12a.5.5 0 00.5.5h4.25V9.75H1.5zm6.25 2.75H12a.5.5 0 00.5-.5V9.75H7.75v2.75zm4.75-4.25v-2.5H7.75v2.5h4.75z"
        fill="currentColor"
        fillRule="evenodd"
      />
    </svg>
  );
}

// NavSuperscript from popsql-lollipop. Used for function/procedure groups.
export function NavSuperscript({ className = '', size = 14 }: IconProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 14 14"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M11.0625 5.07812V4.21875H8.20312V5.04688H9.625L7.59375 7.71875L5.5625 5.04688H7V4.21875H4.125V5.07812H4.6875L6.96875 8.04688L4.65625 11.0469H4.125V11.9062H7V11.0781H5.5625L7.59375 8.40625L9.625 11.0781H8.20312V11.9062H11.0625V11.0469H10.5469L8.21875 8.04688L10.5 5.07812H11.0625Z"
        fill="currentColor"
      />
      <path
        d="M11.0938 2.32812L11.4688 2.10938C11.6354 2.01562 11.7656 1.91146 11.8594 1.79688C11.9531 1.67708 12 1.55208 12 1.42188C12 1.27604 11.9479 1.16146 11.8438 1.07812C11.7448 0.989583 11.6094 0.945312 11.4375 0.945312C11.2917 0.945312 11.1641 0.979167 11.0547 1.04688C10.9453 1.10938 10.8594 1.19531 10.7969 1.30469L10.4844 1.10938C10.5781 0.953125 10.7083 0.828125 10.875 0.734375C11.0469 0.640625 11.2448 0.59375 11.4688 0.59375C11.7031 0.59375 11.8932 0.661458 12.0391 0.796875C12.1849 0.927083 12.2578 1.09635 12.2578 1.30469C12.2578 1.46094 12.2161 1.61458 12.1328 1.76562C12.0547 1.91146 11.9219 2.05208 11.7344 2.1875L11.1094 2.625H12.3438V3H10.4844V2.6875L11.0938 2.32812Z"
        fill="currentColor"
      />
    </svg>
  );
}

// "Schema" icon: a stylised hierarchical tree (3 small boxes connected to
// a root). Sourced from popsql-lollipop's `schema.svg`.
export function SchemaIcon({ className = '', size = 14 }: IconProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M9.49992 12.0303H6.49992V15.0303H9.49992V12.0303Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M9.49992 1.00003H6.49992V4.00003H9.49992V1.00003Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M14.9999 12H11.9999V15H14.9999V12Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M3.99992 12H0.99992V15H3.99992V12Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M8 6.27271V9.72725"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M2.81818 9.72727V8H13.1818V9.72727"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function RefreshIcon({ className = '', size = 14 }: IconProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 12a9 9 0 0 1 15.5-6.4L21 8" />
      <path d="M21 3v5h-5" />
      <path d="M21 12a9 9 0 0 1-15.5 6.4L3 16" />
      <path d="M3 21v-5h5" />
    </svg>
  );
}
