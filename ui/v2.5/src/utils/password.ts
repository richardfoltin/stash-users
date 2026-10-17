// bcrypt, which is used to store passwords, cannot use longer passwords
export const maxPasswordBytes = 72;

// returns true if the password is longer than the maximum length in bytes
export function passwordTooLong(password: string) {
  return new TextEncoder().encode(password).length > maxPasswordBytes;
}
