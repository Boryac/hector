'use client';

import React, { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Skeleton } from './ui';

// Legacy settings URLs are kept as deep links into the unified settings page.
export function SettingsRedirect({ section }: { section: string }) {
  const router = useRouter();

  useEffect(() => {
    router.replace(`/settings#section-${section}`);
  }, [router, section]);

  return (
    <div className="page">
      <Skeleton height={220} />
    </div>
  );
}