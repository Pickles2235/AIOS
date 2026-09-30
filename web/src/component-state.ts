export type ButtonVariant = "primary" | "secondary" | "quiet";
export type NoticeTone = "info" | "error";

export const buttonVariant = (variant: ButtonVariant = "secondary") => variant;
export const noticeRole = (tone: NoticeTone) => tone === "error" ? "alert" : undefined;
