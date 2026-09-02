import { createContext, ReactNode, useContext } from 'react';

const PermissionsContext = createContext<string[]>(['*']);

export function PermissionsProvider({ permissions, children }: { permissions: string[]; children: ReactNode }) {
  return <PermissionsContext.Provider value={permissions}>{children}</PermissionsContext.Provider>;
}

export function useCan(permission: string) {
  const permissions = useContext(PermissionsContext);
  return permissions.includes('*') || permissions.includes(permission);
}
