type IconName =
  | 'arrow-left'
  | 'arrow-right'
  | 'arrow-up'
  | 'arrow-down'
  | 'check'
  | 'x'
  | 'circle'
  | 'circle-check'
  | 'circle-x'
  | 'square'
  | 'square-check'
  | 'chevron-down'
  | 'chevron-right'
  | 'plus'
  | 'minus'
  | 'search'
  | 'filter'
  | 'copy'
  | 'refresh'
  | 'pause'
  | 'play'
  | 'stop'
  | 'inbox'
  | 'home'
  | 'settings'
  | 'help'
  | 'spinner';

interface IconProps {
  name: IconName;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
  'aria-label'?: string;
}

// Simple SVG icon paths - keeping it minimal and consistent
const icons: Record<IconName, string> = {
  'arrow-left': 'M15 19l-7-7 7-7',
  'arrow-right': 'M9 5l7 7-7 7',
  'arrow-up': 'M5 15l7-7 7 7',
  'arrow-down': 'M19 9l-7 7-7-7',
  'check': 'M5 12l5 5L20 7',
  'x': 'M6 18L18 6M6 6l12 12',
  'circle': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0',
  'circle-check': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0M9 12l2 2 4-4',
  'circle-x': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0M10 10l4 4m0-4l-4 4',
  'square': 'M3 3h18v18H3z',
  'square-check': 'M3 3h18v18H3zM9 12l2 2 4-4',
  'chevron-down': 'M6 9l6 6 6-6',
  'chevron-right': 'M9 6l6 6-6 6',
  'plus': 'M12 5v14m-7-7h14',
  'minus': 'M5 12h14',
  'search': 'M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z',
  'filter': 'M3 4h18l-7 8v6l-4 2V12L3 4z',
  'copy': 'M8 4H6a2 2 0 00-2 2v12a2 2 0 002 2h8a2 2 0 002-2v-2M16 4h2a2 2 0 012 2v12M12 12H8m4 4H8m8-8h4',
  'refresh': 'M4 4v5h5M20 20v-5h-5M5 10a7 7 0 0113.5-2M19 14a7 7 0 01-13.5 2',
  'pause': 'M6 4h4v16H6zM14 4h4v16h-4z',
  'play': 'M5 3l14 9-14 9V3z',
  'stop': 'M6 6h12v12H6z',
  'inbox': 'M3 8l4 8h10l4-8M5 16v4h14v-4',
  'home': 'M3 12l9-9 9 9M5 10v10h14V10',
  'settings': 'M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 11-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 110-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 112.83-2.83l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 114 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 112.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9c.26.604.852.997 1.51 1H21a2 2 0 110 4h-.09a1.65 1.65 0 00-1.51 1z',
  'help': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0M9 9a3 3 0 015.12 2.12c0 1.38-1.62 2.38-2.62 2.88M12 17h.01',
  'spinner': 'M12 2v4m0 12v4m10-10h-4M6 12H2m15.07-5.07l-2.83 2.83M9.76 14.24l-2.83 2.83m11.14 0l-2.83-2.83M9.76 9.76L6.93 6.93',
};

const sizes = {
  sm: 16,
  md: 20,
  lg: 24,
};

export function Icon({ name, size = 'md', className = '', 'aria-label': ariaLabel }: IconProps) {
  const pixelSize = sizes[size];
  const path = icons[name];

  if (!path) {
    console.warn(`Icon "${name}" not found`);
    return null;
  }

  return (
    <svg
      className={`app-icon app-icon--${size} ${name === 'spinner' ? 'app-icon--spinning' : ''} ${className}`}
      width={pixelSize}
      height={pixelSize}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={!ariaLabel}
      aria-label={ariaLabel}
      role={ariaLabel ? 'img' : undefined}
    >
      <path d={path} />
    </svg>
  );
}
