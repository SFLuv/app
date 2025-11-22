# Docker Setup for SFLuv Backend

This guide explains how to run the SFLuv backend application using Docker and Docker Compose.

## Prerequisites

- Docker
- Docker Compose

## Quick Start

1. **Clone the repository and navigate to the backend directory**
   ```bash
   cd /path/to/sfluv/app/backend
   ```

2. **Create your environment file**
   ```bash
   cp .env.example .env
   ```
   
   Edit `.env` with your actual values:
   ```bash
   nano .env
   ```

3. **Start the services**
   ```bash
   docker-compose up -d
   ```

4. **Check the logs**
   ```bash
   docker-compose logs -f backend
   ```

5. **Access the application**
   - Backend API: http://localhost:8080
   - Health check: http://localhost:8080/health
   - PostgreSQL: localhost:5432
   - Redis: localhost:6379

## Services

### Backend Application
- **Container name**: `sfluv-backend`
- **Port**: 8080
- **Health check**: `/health` endpoint

### PostgreSQL Database
- **Container name**: `sfluv-postgres`
- **Port**: 5432
- **Default credentials**: See `.env.example`
- **Persistent storage**: Named volume `postgres_data`

### Redis Cache
- **Container name**: `sfluv-redis`
- **Port**: 6379
- **Persistent storage**: Named volume `redis_data`

## Commands

### Development Commands

```bash
# Start all services in background
docker-compose up -d

# Start with logs visible
docker-compose up

# Stop all services
docker-compose down

# Stop and remove volumes (WARNING: This will delete all data)
docker-compose down -v

# Rebuild backend image
docker-compose build backend

# Force rebuild without cache
docker-compose build --no-cache backend

# View logs
docker-compose logs backend
docker-compose logs postgres
docker-compose logs redis

# Follow logs
docker-compose logs -f backend

# Execute commands in running containers
docker-compose exec backend sh
docker-compose exec postgres psql -U sfluv_user -d sfluv_db
docker-compose exec redis redis-cli
```

### Database Commands

```bash
# Connect to PostgreSQL
docker-compose exec postgres psql -U sfluv_user -d sfluv_db

# Create database backup
docker-compose exec postgres pg_dump -U sfluv_user sfluv_db > backup.sql

# Restore database from backup
docker-compose exec -T postgres psql -U sfluv_user -d sfluv_db < backup.sql
```

### Maintenance Commands

```bash
# View resource usage
docker-compose top

# Check service health
docker-compose ps

# Clean up unused Docker resources
docker system prune

# Remove stopped containers
docker container prune

# Remove unused images
docker image prune

# Remove unused volumes (BE CAREFUL)
docker volume prune
```

## Environment Variables

The following environment variables need to be set in your `.env` file:

### Required
- `PORT`: Server port (default: 8080)
- `DB_USER`: Database user
- `DB_PASSWORD`: Database password
- `DB_NAME`: Database name
- `ADMIN_KEY`: Admin authentication key

### Blockchain/Web3
- `TOKEN_ID`: Token contract ID
- `TOKEN_DECIMALS`: Token decimals
- `RPC_URL`: Blockchain RPC URL
- `BOT_KEY`: Bot private key

### Authentication
- `PRIVY_APP_ID`: Privy application ID
- `PRIVY_VKEY`: Privy verification key

## Troubleshooting

### Common Issues

1. **Port already in use**
   ```bash
   # Check what's using the port
   lsof -i :8080
   # Kill the process or change the port in .env
   ```

2. **Database connection issues**
   ```bash
   # Check if PostgreSQL is running
   docker-compose ps postgres
   # Check PostgreSQL logs
   docker-compose logs postgres
   ```

3. **Permission issues with logs**
   ```bash
   # Create logs directory with proper permissions
   mkdir -p logs/prod logs/test/app
   chmod 755 logs logs/prod logs/test logs/test/app
   ```

4. **Build issues**
   ```bash
   # Clean rebuild
   docker-compose down
   docker-compose build --no-cache
   docker-compose up
   ```

### Health Checks

The application includes health checks for monitoring:

- **Backend**: HTTP GET to `/health`
- **PostgreSQL**: `pg_isready` command
- **Redis**: Built-in health check

Check health status:
```bash
docker-compose ps
```

### Logs Location

- **Application logs**: `./logs/prod/app.log`
- **Docker logs**: `docker-compose logs [service_name]`

## Production Considerations

1. **Security**
   - Change default passwords
   - Use secrets management
   - Enable SSL/TLS
   - Restrict network access

2. **Performance**
   - Adjust resource limits
   - Configure connection pooling
   - Set up monitoring

3. **Backup**
   - Regular database backups
   - Persistent volume snapshots
   - Log rotation

4. **Scaling**
   - Use orchestration (Kubernetes)
   - Load balancing
   - Database clustering