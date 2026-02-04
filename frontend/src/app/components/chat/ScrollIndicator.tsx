interface ScrollIndicatorProps {
  visible: boolean;
  onClick: () => void;
}

export function ScrollIndicator({ visible, onClick }: ScrollIndicatorProps) {
  if (!visible) return null;

  return (
    <button
      type="button"
      className="app-scroll-indicator"
      onClick={onClick}
    >
      ↓ New messages
    </button>
  );
}
