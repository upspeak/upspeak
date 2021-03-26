use anyhow::Result;
use sqlx::postgres::{PgPool, PgPoolOptions};

pub struct PgStore {
  pgpool: PgPool,
}

impl PgStore {
  pub async fn new(connstr: &str) -> Result<Self> {
    let pool = PgPoolOptions::new()
      .max_connections(5)
      .connect(&connstr)
      .await?;
    Ok(PgStore { pgpool: pool })
  }
  pub fn pool(&self) -> &PgPool {
    &self.pgpool
  }
}
