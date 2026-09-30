// Client notices use the same catalog and selected language as the Go templates.
window.CongridI18n = {
  t(message, ...args) {
    const text = window.CongridMessages?.[message] || message;
    return text.replace(/\{(\d+)\}/g, (match, index) =>
      index < args.length ? String(args[index]) : match);
  },
};
