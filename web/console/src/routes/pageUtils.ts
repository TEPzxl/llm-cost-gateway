import type { FormEvent } from "react";

export type PageProps = {
  client: import("../api/client").ApiClient;
};

export function formValue(form: HTMLFormElement, name: string) {
  const value = new FormData(form).get(name);
  return typeof value === "string" ? value.trim() : "";
}

export function numberValue(form: HTMLFormElement, name: string, fallback = 0) {
  const value = Number(formValue(form, name));
  return Number.isFinite(value) ? value : fallback;
}

export function preventDefault(event: FormEvent<HTMLFormElement>) {
  event.preventDefault();
  return event.currentTarget;
}

export function formatDate(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}

export function formatMicroUSD(value: number) {
  return `$${(value / 1_000_000).toFixed(6)}`;
}
