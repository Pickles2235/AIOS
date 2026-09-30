export async function createCustomer(body: unknown) {
  return axios.post("/api/v1/customers", body);
}
