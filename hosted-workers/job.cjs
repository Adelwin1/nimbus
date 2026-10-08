const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function jobOptions(env = process.env) {
 const once = env.NIMBUS_JOB_ID !== undefined;
 if (once && !UUID.test(env.NIMBUS_JOB_ID)) throw new Error("Invalid hosted job ID.");
 return { once, id: once ? env.NIMBUS_JOB_ID : null };
}
module.exports = { jobOptions };
