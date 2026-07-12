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
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.post('/auth/logout', {}),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}

export function useSetupAuth() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (password: string) =>
      api.post('/auth/setup', { password }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}

export function useChangePassword() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (params: { currentPassword: string; newPassword: string }) =>
      api.post('/auth/password', {
        current_password: params.currentPassword,
        new_password: params.newPassword,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}
