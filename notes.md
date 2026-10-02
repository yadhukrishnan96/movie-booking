
❯  podman run -d \
  --name movie-booking \ 
  -e POSTGRES_USER=movie \    
  -e POSTGRES_PASSWORD=movie \    
  -e POSTGRES_DB=moviebooking \ 
  -p 5432:5432 \
  postgres:18  