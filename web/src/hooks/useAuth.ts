import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';

type AuthStatus = {
  enabled: boolean;
  authenticated: boolean;
};

export const authKeys = {
  all: ['auth'] as const,
  status: ['auth', 'status'] as const,
};

export function useAuth() {
  const { data: auth, isLoading } = useQuery<AuthStatus>({
    queryKey: authKeys.status,
    queryFn: () => api.get('/auth/status'),
  });

  return { auth, isLoading };
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (password: string) =>
      api.post('/auth/login', { password }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.post('/auth/logout', {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}
