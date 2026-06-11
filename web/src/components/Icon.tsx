import type { FC, SVGProps } from 'react';

// Each icon is a separate .svg file under ../assets/icons, normalized to a
// 16x16 viewBox and using `currentColor` so it inherits color from CSS.
// vite-plugin-svgr turns each `?react` import into a React component, and
// import.meta.glob loads them all by name.
const iconModules = import.meta.glob<FC<SVGProps<SVGSVGElement>>>(
  '../assets/icons/*.svg',
  { eager: true, query: '?react', import: 'default' },
);

const icons = Object.fromEntries(
  Object.entries(iconModules).map(([path, component]) => {
    const name = path.split('/').pop()?.replace('.svg', '') ?? path;
    return [name, component];
  }),
) as Record<IconName, FC<SVGProps<SVGSVGElement>>>;

// IconName enumerates the available icons (the .svg filenames). Keeping this an
// explicit union gives compile-time safety at call sites.
export type IconName =
  | 'chevron-down'
  | 'check'
  | 'comment'
  | 'copy'
  | 'eye'
  | 'eye-off'
  | 'function'
  | 'new-query'
  | 'refresh'
  | 'table'
  | 'x';

export type IconColor =
  | 'current'
  | 'gray'
  | 'green'
  | 'red'
  | 'blue'
  | 'orange'
  | 'yellow'
  | 'white';

const iconColorToClass: Record<IconColor, string> = {
  current: 'text-current',
  gray: 'text-slate-500',
  green: 'text-green-600',
  red: 'text-red-600',
  blue: 'text-blue-600',
  orange: 'text-orange-600',
  yellow: 'text-yellow-600',
  white: 'text-white',
};

export type IconSize = 'lg' | 'md' | 'sm' | 'xs';

const iconSizeToPx: Record<IconSize, number> = {
  lg: 32,
  md: 24,
  sm: 16,
  xs: 12,
};

export interface IconProps {
  name: IconName;
  className?: string;
  color?: IconColor;
  size?: IconSize | number;
  width?: number;
  height?: number;
  style?: React.CSSProperties;
}

export function Icon({
  name,
  className = '',
  color = 'current',
  size = 'sm',
  width,
  height,
  style,
}: IconProps) {
  const Svg = icons[name];
  if (!Svg) return null;

  const px = typeof size === 'number' ? size : iconSizeToPx[size];

  return (
    <Svg
      role="img"
      aria-hidden="true"
      width={width ?? px}
      height={height ?? px}
      style={style}
      className={`flex-shrink-0 ${iconColorToClass[color]} ${className}`.trim()}
    />
  );
}
