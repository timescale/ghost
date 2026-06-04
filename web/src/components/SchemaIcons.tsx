// SVG icons used by the schema tree. Sourced from the popsql lollipop
// beta-icon set so the appearance matches popsql exactly.

export interface IconProps {
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

// CopyIcon from popsql-lollipop (16x16). Used for all "Copy" context menu items.
export function CopyIcon({ className = '', size = 16 }: IconProps) {
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
        d="M13.333 5.25c1.15 0 2.084.933 2.084 2.083v6c0 1.15-.933 2.084-2.084 2.084h-6a2.083 2.083 0 01-2.083-2.084v-6c0-1.15.933-2.083 2.083-2.083zm0 1.5h-6a.583.583 0 00-.583.583v6c0 .323.261.584.583.584h6a.583.583 0 00.584-.584v-6a.583.583 0 00-.584-.583zM8.667.583a2.083 2.083 0 012.075 1.9l.008.184v.666a.75.75 0 01-1.493.102l-.007-.102v-.666a.583.583 0 00-.492-.576l-.091-.008h-6a.583.583 0 00-.576.492l-.008.092v6a.583.583 0 00.492.576l.092.007h.666a.75.75 0 01.102 1.493l-.102.007h-.666A2.083 2.083 0 01.59 8.85l-.008-.183v-6A2.083 2.083 0 012.483.59l.184-.008h6z"
        fill="currentColor"
        fillRule="nonzero"
      />
    </svg>
  );
}

// EyeIcon from popsql-lollipop (16x16). Used for "View definition" context menu items.
export function EyeIcon({ className = '', size = 16 }: IconProps) {
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
        d="M2.21401 8.37436C2.12204 8.23194 2.04481 8.10543 1.98271 8C2.04481 7.89457 2.12204 7.76806 2.21401 7.62564C2.49923 7.18402 2.92144 6.59705 3.46903 6.01296C4.57789 4.83017 6.11213 3.75 8 3.75C9.88787 3.75 11.4221 4.83017 12.531 6.01296C13.0786 6.59705 13.5008 7.18402 13.786 7.62564C13.878 7.76806 13.9552 7.89457 14.0173 8C13.9552 8.10542 13.878 8.23194 13.786 8.37436C13.5008 8.81598 13.0786 9.40295 12.531 9.98704C11.4221 11.1698 9.88787 12.25 8 12.25C6.11213 12.25 4.57789 11.1698 3.46903 9.98704C2.92144 9.40295 2.49923 8.81598 2.21401 8.37436ZM15.5456 7.66407C15.5457 7.66434 15.5458 7.66459 14.875 8C15.5458 8.33541 15.5457 8.33566 15.5456 8.33593L15.5452 8.33656L15.5444 8.33817L15.5421 8.34275L15.5347 8.35726C15.5285 8.36928 15.5199 8.38595 15.5088 8.40694C15.4865 8.44892 15.4545 8.50828 15.4127 8.58254C15.3292 8.73096 15.2066 8.93949 15.046 9.18814C14.7258 9.68402 14.2496 10.3471 13.6253 11.013C12.3904 12.3302 10.4871 13.75 8 13.75C5.51287 13.75 3.60961 12.3302 2.37472 11.013C1.75044 10.3471 1.27421 9.68402 0.953955 9.18814C0.793364 8.93949 0.67077 8.73096 0.587285 8.58254C0.545515 8.50828 0.513451 8.44892 0.491236 8.40694C0.480127 8.38595 0.471473 8.36928 0.465292 8.35726L0.457877 8.34275L0.455563 8.33817L0.454755 8.33656L0.454438 8.33593C0.454304 8.33566 0.45418 8.33541 1.125 8L0.45418 7.66459L0.454304 7.66434L0.454438 7.66407L0.454755 7.66344L0.455563 7.66183L0.457877 7.65725L0.465292 7.64274C0.471473 7.63072 0.480127 7.61405 0.491236 7.59306C0.513451 7.55108 0.545515 7.49172 0.587285 7.41746C0.67077 7.26904 0.793364 7.06051 0.953955 6.81186C1.27421 6.31598 1.75044 5.65295 2.37472 4.98704C3.60961 3.66983 5.51287 2.25 8 2.25C10.4871 2.25 12.3904 3.66983 13.6253 4.98704C14.2496 5.65295 14.7258 6.31598 15.046 6.81186C15.2066 7.06051 15.3292 7.26904 15.4127 7.41746C15.4545 7.49172 15.4865 7.55108 15.5088 7.59306C15.5199 7.61405 15.5285 7.63072 15.5347 7.64274L15.5421 7.65725L15.5444 7.66183L15.5452 7.66344L15.5456 7.66407ZM14.875 8L15.5458 7.66459C15.6514 7.87574 15.6514 8.12426 15.5458 8.33541L14.875 8ZM0.45418 7.66459L1.125 8L0.45418 8.33541C0.348607 8.12426 0.348607 7.87574 0.45418 7.66459ZM6.875 8C6.875 7.37868 7.37868 6.875 8 6.875C8.62132 6.875 9.125 7.37868 9.125 8C9.125 8.62132 8.62132 9.125 8 9.125C7.37868 9.125 6.875 8.62132 6.875 8ZM8 5.375C6.55025 5.375 5.375 6.55025 5.375 8C5.375 9.44975 6.55025 10.625 8 10.625C9.44975 10.625 10.625 9.44975 10.625 8C10.625 6.55025 9.44975 5.375 8 5.375Z"
        fill="currentColor"
        fillRule="evenodd"
      />
    </svg>
  );
}

// NavQueriesPlus from popsql-lollipop (16x16). Used for "New query" context menu items.
export function NavQueriesPlus({ className = '', size = 16 }: IconProps) {
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
        clipRule="evenodd"
        d="M1.5267 1.27645C1.9174 0.885753 2.4473 0.66626 2.99984 0.66626H8.33317C8.53208 0.66626 8.72285 0.745277 8.8635 0.88593L12.8635 4.88593C13.0042 5.02658 13.0832 5.21735 13.0832 5.41626V8.08292C13.0832 8.49713 12.7474 8.83292 12.3332 8.83292C11.919 8.83292 11.5832 8.49713 11.5832 8.08292V6.16626H8.33325C7.91904 6.16626 7.58325 5.83048 7.58325 5.41626V2.16626H2.99984C2.84513 2.16626 2.69675 2.22772 2.58736 2.33711C2.47796 2.44651 2.4165 2.59488 2.4165 2.74959V13.4163C2.4165 13.571 2.47796 13.7193 2.58736 13.8287C2.69676 13.9381 2.84513 13.9996 2.99984 13.9996H8.33317C8.74739 13.9996 9.08317 14.3354 9.08317 14.7496C9.08317 15.1638 8.74739 15.4996 8.33317 15.4996H2.99984C2.4473 15.4996 1.9174 15.2801 1.5267 14.8894C1.136 14.4987 0.916504 13.9688 0.916504 13.4163V2.74959C0.916504 2.19706 1.136 1.66715 1.5267 1.27645ZM9.08325 3.227L10.5225 4.66626H9.08325V3.227ZM4.49988 4.68292C4.08566 4.68292 3.74988 5.01871 3.74988 5.43292C3.74988 5.84714 4.08566 6.18292 4.49988 6.18292H5.49988C5.91409 6.18292 6.24988 5.84714 6.24988 5.43292C6.24988 5.01871 5.91409 4.68292 5.49988 4.68292H4.49988ZM3.74988 8.58292C3.74988 8.1687 4.08566 7.83292 4.49988 7.83292H9.49988C9.91409 7.83292 10.2499 8.1687 10.2499 8.58292C10.2499 8.99713 9.91409 9.33292 9.49988 9.33292H4.49988C4.08566 9.33292 3.74988 8.99713 3.74988 8.58292ZM4.49988 10.8329C4.08566 10.8329 3.74988 11.1687 3.74988 11.5829C3.74988 11.9971 4.08566 12.3329 4.49988 12.3329H7.49988C7.91409 12.3329 8.24988 11.9971 8.24988 11.5829C8.24988 11.1687 7.91409 10.8329 7.49988 10.8329H4.49988Z"
        fill="currentColor"
        fillRule="evenodd"
      />
      <path
        d="M13.323 10.76V14.76"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
      <path
        d="M15.323 12.76H11.323"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}
