/**
 * True in the public demo build (web/Dockerfile.demo): the pages hide what a visitor cannot
 * do. Hiding is a courtesy; the demo's nginx is what refuses the requests.
 */
export const READ_ONLY = import.meta.env.VITE_READ_ONLY === "1";
