use std::convert::Infallible;
use std::env;
use std::net::SocketAddr;

use auth::password::calculate_password_hash;
use http::set_header::set_header;
use hyper::service::{make_service_fn, service_fn};
use hyper::{Body, Request, Response, Server, StatusCode};

use crate::auth::password::set_password;
use crate::data::rsa_keypair::RsaKeypair;
use crate::db::relying_party::register_relying_party;
use crate::db::rsa_keypair::insert_ignore;

mod auth;
mod crypto;
mod dash;
mod data;
mod db;
mod enc;
mod external;
mod http;
mod mail;
mod oidc;
mod time;

fn create_admin_user() {
    let user_id = env::var("ADMIN_USER").unwrap();
    let password = env::var("ADMIN_PASSWORD").unwrap();

    let user = data::user::User { id: user_id };
    let create_user = db::user::insert_user(&user);
    if create_user.is_err() {
        println!("Skipped creating admin user.");
        return;
    }
    set_password(&user.id, &password);
    println!("Admin user created.");
}

fn create_default_rp() {
    let client_secret = env::var("CLIENT_SECRET").unwrap();
    let client_secret = calculate_password_hash(&client_secret, "accounts.comame.xyz");
    let result = register_relying_party("accounts.comame.xyz", &client_secret);
    if result.is_err() {
        dbg!("Skipped create default RP.");
    }
    let result = crate::db::relying_party::add_redirect_uri(
        "accounts.comame.xyz",
        &format!("{}/rp/callback", env::var("HOST").unwrap()),
    );
    if result.is_err() {
        dbg!("Default RP redirect_uri is already set.");
    }
    let result = crate::db::relying_party::add_redirect_uri(
        "accounts.comame.xyz",
        &format!("{}/dash/callback", env::var("HOST").unwrap()),
    );
    if result.is_err() {
        dbg!("Default RP redirect_uri is already set.");
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
