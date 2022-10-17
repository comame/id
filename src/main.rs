use std::convert::Infallible;
use std::env;
use std::net::SocketAddr;

use http::set_header::set_header;
use hyper::service::{make_service_fn, service_fn};
use hyper::{Body, Request, Response, Server, StatusCode};
use mysql::params;
use mysql::prelude::*;

use crate::auth::password::set_password;
use crate::data::rsa_keypair::RsaKeypair;
use crate::db::rsa_keypair::insert_ignore;

mod auth;
mod crypto;
mod data;
mod db;
mod enc;
mod http;
mod oidc;
mod time;

fn create_admin_user() {
    let user_id = env::var("ADMIN_USER").unwrap();
    let password = env::var("ADMIN_PASSWORD").unwrap();

    let user = data::user::User { id: user_id };
    let create_user = db::user::insert_user(&user);
    if let Err(err) = create_user {
        println!("{}", err);
        println!("Skipped creating admin user.");
        return;
    }
    set_password(&user.id, &password);
    println!("Admin user created.");
}

fn create_default_rp() {
    crate::db::mysql::get_conn()
        .unwrap()
        .exec_drop(
            "DELETE FROM relying_parties WHERE client_id = :id",
            params! { "id" => "id.comame.dev"},
        )
        .unwrap();
    let secret = crate::data::oidc_relying_party::RelyingParty::register("id.comame.dev");
    let _res = crate::db::relying_party::add_redirect_uri(
        "id.comame.dev",
        "http://localhost:8080/rp/callback",
    );
    if let Ok(secret) = secret {
        dbg!(secret);
    }
}

fn moved_permanently(path: &str) -> Response<Body> {
    let mut response = Response::new(Body::empty());
    *response.status_mut() = StatusCode::MOVED_PERMANENTLY;
    set_header(&mut response, "Location", path);
    response
}

async fn service(req: Request<Body>) -> Result<Response<Body>, Infallible> {
    Ok(match http::uri::trim(req.uri().path()) {
        Some(path) => moved_permanently(path.as_str()),
        None => http::routes::routes(req).await,
    })
}

#[tokio::main]
async fn main() {
    let mysql_user = env::var("MYSQL_USER").unwrap();
    let mysql_password = env::var("MYSQL_PASSWORD").unwrap();
    let mysql_db = env::var("MYSQL_DATABASE").unwrap();
    let mysql_host = env::var("MYSQL_HOST").unwrap();
    db::mysql::init(&format!(
        "mysql://{}:{}@{}/{}",
        mysql_user, mysql_password, mysql_host, mysql_db
    ));

    create_admin_user();

    create_default_rp();

    insert_ignore(&RsaKeypair::new());
    dbg!(RsaKeypair::get().public);

    let redis_host = env::var("REDIS_HOST").unwrap();
    db::redis::init(&format!("redis://{}", redis_host));

    let addr = SocketAddr::from(([0, 0, 0, 0], 8080));
    let make_service = make_service_fn(|_conn| async { Ok::<_, Infallible>(service_fn(service)) });
    let serve = Server::bind(&addr).serve(make_service);
    let result = tokio::spawn(serve);
    println!("Server is listening on {}", addr);
    if let Err(err) = result.await {
        eprintln!("Server error: {}", err);
    };
}
