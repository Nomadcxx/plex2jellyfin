/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  distDir: 'out',
  trailingSlash: true,

  images: {
    unoptimized: true,
  },

  async rewrites() {
    return process.env.NODE_ENV === 'development'
      ? [
          {
            source: '/api/:path*',
            destination: 'http://localhost:8686/api/:path*',
          },
        ]
      : [];
  },
};

module.exports = nextConfig;
