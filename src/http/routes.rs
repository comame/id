use hyper::{Body, Method, Request, Response, StatusCode};

use crate::http::{handler, static_file};

pub async fn routes(req: Request<Body>) -> Response<Body> {
    let start_time = std::time::SystemTime::now();

    let mut response = Response::new(Body::empty());

    let uri = req.uri().clone();
    let method = req.method().clone();

    match (req.method(), req.uri().path()) {
        (&Method::GET, "/signin") => {
            response = handler::signin::page("signin");
        }
        (&Method::GET, "/reauthenticate") => {
            response = handler::signin::page("reauthenticate");
        }
        (&Method::GET, "/confirm") => {
            response = handler::signin::page("confirm");
        }
        (&Method::GET, "/signout") => {
            response = handler::signout::signout(req).await;
        }
        (&Method::POST, "/api/signin-password") => {
            response = handler::signin::sign_in_with_password(req).await;
        }
        (&Method::POST, "/api/signin-session") => {
            response = handler::signin::sign_in_with_session(req).await;
        }
        (&Method::POST, "/api/signin-continue") => {
            response = handler::signin_continue::handler(req).await;
        }
        (&Method::POST, "/api/signin-continue-nointeraction-fail") => {
            response = handler::signin_continue::no_interaction_fail(req).await;
        }
        (&Method::GET, "/authenticate") => {
            response = handler::oidc_authentication_request::handler(req).await;
        }
        (&Method::POST, "/authenticate") => {
            response = handler::oidc_authentication_request::handler(req).await;
        }
        (&Method::POST, "/code") => {
            response = handler::oidc_code_request::handle(req).await;
        }
        (&Method::GET, "/rp/callback") => {
            response = handler::rp_callback::handler(req).await;
        }
        _ => {
            let file = static_file::read(req.uri().path());

            if file.is_ok() {
                *response.body_mut() = Body::from(file.unwrap());
            } else {
                *response.body_mut() = "Not Found".into();
                *response.status_mut() = StatusCode::NOT_FOUND;
            }
        }
    };

    let time = std::time::SystemTime::now()
        .duration_since(start_time)
        .unwrap()
        .as_millis();

    println!("REQ {method} {} {time}", uri.path());

    response
}
